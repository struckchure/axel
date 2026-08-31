package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
	"github.com/struckchure/axel/generators/typescript"
)

const multiCodegenSchema = `
enum UserType { Admin, Runner, Vendor }

type User {
  required id: uuid { constraint pk };
  required email: str;
  multi roles: UserType;
  multi images: str;
}

type Vendor {
  required id: uuid { constraint pk };
  required name: str;
  multi link members: User;
}`

// A `multi` scalar is an array column, so it must type as an array in the models
// and in query result rows — in both generators. The nullability has to land
// outside the array (`T[] | null`, not `(T | null)[]`).
func TestMultiScalarTypesAsArray(t *testing.T) {
	ir := parseSchema(t, multiCodegenSchema)
	schema := codegen.FromSchemaIR(ir)
	q := buildQueryDesc(t, ir, "listRoles", "list_roles.aql", `multi select User { id, roles, images };`)
	queries := []codegen.QueryDescriptor{q}

	tsDir := t.TempDir()
	if err := codegen.Walk(schema, queries, &typescript.TsGenerator{}, &codegen.Context{OutDir: tsDir}); err != nil {
		t.Fatalf("ts walk: %v", err)
	}
	models := readFile(t, filepath.Join(tsDir, "models.ts"))
	for _, want := range []string{
		"roles?: UserType[] | null;",
		"images?: string[] | null;",
	} {
		if !strings.Contains(models, want) {
			t.Errorf("models.ts missing %q:\n%s", want, models)
		}
	}
	row := readFile(t, filepath.Join(tsDir, "list_roles.ts"))
	for _, want := range []string{"roles: UserType[] | null;", "images: string[] | null;"} {
		if !strings.Contains(row, want) {
			t.Errorf("query row missing %q:\n%s", want, row)
		}
	}

	goDir := t.TempDir()
	if err := codegen.Walk(schema, queries, &golang.GoGenerator{}, &codegen.Context{OutDir: goDir}); err != nil {
		t.Fatalf("go walk: %v", err)
	}
	// The Go emitter aligns struct tags, so match on the field/type pair only.
	goWant := []string{`Roles`, `[]UserType`, `Images`, `[]string`}
	goModels := readFile(t, filepath.Join(goDir, "models.go"))
	for _, want := range goWant {
		if !strings.Contains(goModels, want) {
			t.Errorf("models.go missing %q:\n%s", want, goModels)
		}
	}
	goQueries := readFile(t, filepath.Join(goDir, "list_roles.go"))
	for _, want := range goWant {
		if !strings.Contains(goQueries, want) {
			t.Errorf("generated Go row missing %q:\n%s", want, goQueries)
		}
	}
}

// Multi links used to be dropped from the TS client entirely. They now appear as
// branded Relation fields, are stripped from Insertable (a junction can't be
// written through an INSERT column list), and the runtime can build the
// junction subquery that selects them.
func TestMultiLinkInTsClient(t *testing.T) {
	ir := parseSchema(t, multiCodegenSchema)
	schema := codegen.FromSchemaIR(ir)

	dir := t.TempDir()
	if err := codegen.Walk(schema, nil, &typescript.TsGenerator{}, &codegen.Context{OutDir: dir}); err != nil {
		t.Fatalf("ts walk: %v", err)
	}

	models := readFile(t, filepath.Join(dir, "models.ts"))
	for _, want := range []string{
		"export type Relation<T> = T[] & { readonly [_axelRelation]: true };",
		"export type RelationKeys<T> = {",
		"members?: Relation<User>;",
	} {
		if !strings.Contains(models, want) {
			t.Errorf("models.ts missing %q:\n%s", want, models)
		}
	}

	runner := readFile(t, filepath.Join(dir, "runner.ts"))
	for _, want := range []string{
		// relation keys are not insertable columns
		`type Insertable<T> = Omit<T, "id" | "createdAt" | "updatedAt" | RelationKeys<T>>;`,
		// a selected relation resolves to the target rows, not the branded type
		"IsRelation<T[K]> extends true ? RelationRows<T[K]> : T[K]",
		// the junction subquery itself
		"function _buildLinkSubSelectSQL(",
		`JOIN "${target.table}" ${t} ON ${t}."${joinField}" = ${jt}."${target.table}"`,
		`COALESCE(json_agg(row_to_json(${sub})), '[]')`,
		// shape keys now consult links
		"const link = type.links?.find((l) => l.is_multi",
	} {
		if !strings.Contains(runner, want) {
			t.Errorf("runner.ts missing %q", want)
		}
	}

	// The junction metadata the runtime needs must survive into the embedded
	// schema. join_field is omitted when it defaults to id (the runtime supplies
	// the default), so it is not asserted here.
	for _, want := range []string{`"junction_table":"vendor_members"`, `"target_type":"User"`} {
		if !strings.Contains(runner, want) {
			t.Errorf("embedded schema JSON missing %q", want)
		}
	}
}

// index.ts re-exports every generated module so consumers can import from the
// output directory itself.
func TestTsBarrelFile(t *testing.T) {
	ir := parseSchema(t, multiCodegenSchema)
	schema := codegen.FromSchemaIR(ir)
	queries := []codegen.QueryDescriptor{
		buildQueryDesc(t, ir, "listRoles", "list_roles.aql", `multi select User { id, roles };`),
		buildQueryDesc(t, ir, "listVendors", "list_vendors.aql", `multi select Vendor { id, name };`),
	}

	dir := t.TempDir()
	if err := codegen.Walk(schema, queries, &typescript.TsGenerator{}, &codegen.Context{OutDir: dir}); err != nil {
		t.Fatalf("ts walk: %v", err)
	}

	barrel := readFile(t, filepath.Join(dir, "index.ts"))
	for _, want := range []string{
		`export * from "./models.ts";`,
		`export * from "./runner.ts";`,
		`export * from "./list_roles.ts";`,
		`export * from "./list_vendors.ts";`,
	} {
		if !strings.Contains(barrel, want) {
			t.Errorf("index.ts missing %q:\n%s", want, barrel)
		}
	}
}
