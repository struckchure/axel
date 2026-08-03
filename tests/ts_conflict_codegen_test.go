package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/typescript"
)

// The generated TypeScript client must:
//   - expose an `unlessConflict` upsert builder whose `else` arm is a
//     `db.update(...)` builder, lowered to ON CONFLICT DO NOTHING / DO UPDATE SET;
//   - expose a standalone `update()` builder on the Runner;
//   - reflect the ASL schema's required/optional fields in the model interface.
func TestTsConflictCodegen(t *testing.T) {
	ir := parseSchema(t, `
abstract type Base {
  required id: uuid { default := gen_uuid(); constraint pk; };
}
type User extending Base {
  required email: str { constraint exclusive; };
  name: str;
  active: bool { default := true };
}`)
	schema := codegen.FromSchemaIR(ir)
	q := buildQueryDesc(t, ir, "listUsers", "list_users.aql", `multi select User { email };`)

	dir := t.TempDir()
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &typescript.TsGenerator{}, &codegen.Context{OutDir: dir}); err != nil {
		t.Fatalf("ts walk: %v", err)
	}

	runner := readFile(t, filepath.Join(dir, "runner.ts"))
	for _, want := range []string{
		// conflict builder + else = update builder
		"unlessConflict(fn?: (t: ConflictColumns<T>) => ConflictSpec<T>): this",
		"type ConflictElse<T> = UpdateBuilder<T> | UpdateFilterChain<T>;",
		"else?: ConflictElse<T> | null;",
		"update: spec.else ? spec.else._setValues() : undefined",
		"insert += ` ON CONFLICT${target} DO NOTHING`;",
		"insert += ` ON CONFLICT${target} DO UPDATE SET ${sets",
		// standalone update builder + Runner method
		"class UpdateBuilder<T> {",
		"class UpdateFilterChain<T> {",
		"function _buildUpdateSQL(",
		"update<K extends keyof AxelSchema>(",
		"): UpdateBuilder<AxelSchema[K]> {",
		// input types: insert enforces required, update is fully partial
		"type Insertable<T> = Omit<T, \"id\" | \"createdAt\" | \"updatedAt\">;",
		"type Updatable<T> = Partial<Insertable<T>>;",
		// single-statement SQL, no manual transaction (breaks Bun's pooled SQL)
		"const sql = insert + \" RETURNING *\";",
		"const out = sql + \" RETURNING *\";",
		"async one(): Promise<T | null> {", // insert one() nullable for DO NOTHING
	} {
		if !strings.Contains(runner, want) {
			t.Errorf("runner.ts missing %q", want)
		}
	}
	// The insert/update SQL builders must not hand-roll BEGIN/COMMIT.
	if strings.Contains(runner, `"BEGIN;\n"`) {
		t.Errorf("runner.ts still wraps builder SQL in BEGIN/COMMIT")
	}

	// Model reflects required (`:`) vs optional (`?:`) fields, and inherited Base id.
	models := readFile(t, filepath.Join(dir, "models.ts"))
	for _, want := range []string{
		"id: string;",             // required, inherited from Base
		"email: string;",          // required
		"name?: string | null;",   // optional
		"active?: boolean | null;", // optional (has default)
	} {
		if !strings.Contains(models, want) {
			t.Errorf("models.ts missing %q:\n%s", want, models)
		}
	}
}
