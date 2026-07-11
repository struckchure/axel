package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
	"github.com/struckchure/axel/generators/typescript"
)

const directiveSchema = `type User { required id: uuid; required email: str; required name: str; }`

func TestDirectiveParseRoundTrip(t *testing.T) {
	src := "@name CreateUser\n@request CreateUserInput\n@response User\ninsert User { email := $email };"
	stmt, err := aql.ParseString(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dirs := stmt.DirectiveMap()
	for k, want := range map[string]string{"name": "CreateUser", "request": "CreateUserInput", "response": "User"} {
		if dirs[k] != want {
			t.Errorf("directive %q = %q, want %q", k, dirs[k], want)
		}
	}
	// Round-trip preserves the directives.
	out := aql.Print(stmt)
	for _, want := range []string{"@name CreateUser", "@request CreateUserInput", "@response User"} {
		if !strings.Contains(out, want) {
			t.Errorf("printed AQL missing %q:\n%s", want, out)
		}
	}
}

func TestDirectiveNameSetsQueryName(t *testing.T) {
	ir := parseSchema(t, directiveSchema)
	// Pass an empty name; the @name directive must win over the file-derived name.
	desc := buildQueryDesc(t, ir, "", "some_file.aql", "@name FetchUser\nselect User { id } filter .id = $id<uuid>;")
	if desc.Name != "FetchUser" {
		t.Errorf("query name = %q, want FetchUser", desc.Name)
	}
	if v, _ := desc.Directive("name"); v != "FetchUser" {
		t.Errorf("Directive(name) = %q, want FetchUser", v)
	}
}

func TestGoCodegenDirectiveNamedTypes(t *testing.T) {
	ir := parseSchema(t, directiveSchema)
	schema := codegen.FromSchemaIR(ir)

	// Two queries share @response UserView (identical shape) → emitted once.
	q1 := buildQueryDesc(t, ir, "", "list_users.aql", "@response UserView\nmulti select User { id, email };")
	q2 := buildQueryDesc(t, ir, "", "get_user.aql", "@response UserView\nselect User { id, email } filter .id = $id<uuid>;")
	q3 := buildQueryDesc(t, ir, "", "create_user.aql", "@request CreateUserInput\ninsert User { email := $email, name := $name };")

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q1, q2, q3}, &golang.GoGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	models := readFile(t, filepath.Join(dir, "models.go"))
	if strings.Count(models, "type UserView struct") != 1 {
		t.Errorf("UserView should be emitted exactly once in models.go:\n%s", models)
	}
	if !strings.Contains(models, "type CreateUserInput struct") {
		t.Errorf("models.go missing hoisted CreateUserInput:\n%s", models)
	}

	// Query files reference the hoisted names and do not redeclare them.
	list := readFile(t, filepath.Join(dir, "list_users.go"))
	if !strings.Contains(list, "[]UserView") || strings.Contains(list, "type UserView struct") {
		t.Errorf("list_users.go should reference UserView without redeclaring it:\n%s", list)
	}
	create := readFile(t, filepath.Join(dir, "create_user.go"))
	if !strings.Contains(create, "params CreateUserInput") {
		t.Errorf("create_user.go should take CreateUserInput params:\n%s", create)
	}
}

func TestDirectiveResponseConflictAborts(t *testing.T) {
	ir := parseSchema(t, directiveSchema)
	schema := codegen.FromSchemaIR(ir)

	a := buildQueryDesc(t, ir, "", "a.aql", "@response Foo\nselect User { id, email } filter .id = $id<uuid>;")
	b := buildQueryDesc(t, ir, "", "b.aql", "@response Foo\nselect User { id, name } filter .id = $id<uuid>;")

	err := codegen.Walk(schema, []codegen.QueryDescriptor{a, b}, &golang.GoGenerator{}, &codegen.Context{OutDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected conflict error for @response Foo with differing fields")
	}
	for _, want := range []string{"Foo", "a.aql", "b.aql"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("conflict error should mention %q, got: %v", want, err)
		}
	}
}

// A shapeless `@response Application` over a type with enum columns must still
// reuse the schema type: the enum type must appear on both the schema-type and
// result-shape signatures (regression — fieldsSig and typeSig must agree on enums).
func TestDirectiveReuseSchemaTypeWithEnums(t *testing.T) {
	ir := parseSchema(t, enumRowSchema)
	schema := codegen.FromSchemaIR(ir)

	q := buildQueryDesc(t, ir, "", "list_applications.aql",
		"@response Application\nmulti select Application filter .status = $status<ApplicationStatus>?;")
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &golang.GoGenerator{}, &codegen.Context{OutDir: t.TempDir(), Options: map[string]string{"package": "generated"}}); err != nil {
		t.Fatalf("enum-bearing shape should reuse the schema type, got: %v", err)
	}
}

func TestDirectiveReuseSchemaTypeWhenIdentical(t *testing.T) {
	ir := parseSchema(t, directiveSchema)
	schema := codegen.FromSchemaIR(ir)

	// Shapeless select returns all props → matches the User model → reuse, no conflict.
	ok := buildQueryDesc(t, ir, "", "all.aql", "@response User\nmulti select User;")
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{ok}, &golang.GoGenerator{}, &codegen.Context{OutDir: t.TempDir(), Options: map[string]string{"package": "generated"}}); err != nil {
		t.Fatalf("identical shape should reuse the schema type, got: %v", err)
	}

	// A shaped select that omits fields must conflict with the schema type.
	bad := buildQueryDesc(t, ir, "", "partial.aql", "@response User\nselect User { id } filter .id = $id<uuid>;")
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{bad}, &golang.GoGenerator{}, &codegen.Context{OutDir: t.TempDir()}); err == nil {
		t.Fatal("expected conflict when @response name collides with a schema type of a different shape")
	}
}

// An enum-backed result column must generate as the enum union type in TS and be
// imported from models.ts (mirrors the enum-param path).
func TestTsCodegenEnumResultField(t *testing.T) {
	ir := parseSchema(t, enumRowSchema)
	schema := codegen.FromSchemaIR(ir)

	q := buildQueryDesc(t, ir, "", "get_application.aql", `select Application {
      *,
      network := (select Network filter .application = Application.id)
    } filter .id = $id<uuid>;`)

	dir := t.TempDir()
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &typescript.TsGenerator{}, &codegen.Context{OutDir: dir}); err != nil {
		t.Fatalf("walk: %v", err)
	}

	query := readFile(t, filepath.Join(dir, "get_application.ts"))
	if !strings.Contains(query, "status: ApplicationStatus | null") {
		t.Errorf("row interface missing enum-typed status:\n%s", query)
	}
	if !strings.Contains(query, "protocol: NetworkProtocol") {
		t.Errorf("nested row interface missing enum-typed protocol:\n%s", query)
	}
	if !strings.Contains(query, "ApplicationStatus") || !strings.Contains(query, "NetworkProtocol") ||
		!strings.Contains(query, `from "./models.ts"`) {
		t.Errorf("query file should import enum types from models.ts:\n%s", query)
	}
}

// With `--option client=pg`, the TS generator targets node-postgres: query
// functions and the Runner take a pg.Pool and read rows off db.query(...).rows,
// with no Bun DB interface.
func TestTsCodegenPgClient(t *testing.T) {
	ir := parseSchema(t, directiveSchema)
	schema := codegen.FromSchemaIR(ir)

	get := buildQueryDesc(t, ir, "getUser", "get_user.aql", "select User { id, email } filter .id = $id<uuid>;")
	del := buildQueryDesc(t, ir, "delUser", "del_user.aql", "delete User filter .id = $id<uuid>;")

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"client": "pg"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{get, del}, &typescript.TsGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	query := readFile(t, filepath.Join(dir, "get_user.ts"))
	for _, want := range []string{
		`import type { Pool } from "pg";`,
		"getUser(db: Pool,",
		"db.query(query, [params.id]).then((r) => r.rows as GetUserRow[])",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("pg query file missing %q:\n%s", want, query)
		}
	}
	if strings.Contains(query, "db.unsafe") {
		t.Errorf("pg query file should not use Bun's db.unsafe:\n%s", query)
	}

	del2 := readFile(t, filepath.Join(dir, "del_user.ts"))
	if !strings.Contains(del2, "await db.query(query, [params.id]);") {
		t.Errorf("pg delete should call db.query without reading rows:\n%s", del2)
	}

	runner := readFile(t, filepath.Join(dir, "runner.ts"))
	if !strings.Contains(runner, `import type { Pool } from "pg";`) {
		t.Errorf("runner should import Pool from pg:\n%s", runner)
	}
	if strings.Contains(runner, "export interface DB {") {
		t.Errorf("pg runner should not declare the Bun DB interface:\n%s", runner)
	}
	if !strings.Contains(runner, "constructor(private db: Pool)") {
		t.Errorf("Runner should take a pg.Pool:\n%s", runner)
	}
}

// Default (no client option) stays on the Bun DB interface.
func TestTsCodegenDefaultBunClient(t *testing.T) {
	ir := parseSchema(t, directiveSchema)
	schema := codegen.FromSchemaIR(ir)

	get := buildQueryDesc(t, ir, "getUser", "get_user.aql", "select User { id, email } filter .id = $id<uuid>;")
	dir := t.TempDir()
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{get}, &typescript.TsGenerator{}, &codegen.Context{OutDir: dir}); err != nil {
		t.Fatalf("walk: %v", err)
	}
	query := readFile(t, filepath.Join(dir, "get_user.ts"))
	if !strings.Contains(query, "getUser(db: DB,") || !strings.Contains(query, "db.unsafe<GetUserRow>(query, [params.id])") {
		t.Errorf("default client should use the Bun DB interface:\n%s", query)
	}
	if strings.Contains(query, `from "pg"`) {
		t.Errorf("default client should not import pg:\n%s", query)
	}
}

// An invalid client option is a clear error, not silent bad output.
func TestTsCodegenUnknownClientErrors(t *testing.T) {
	ir := parseSchema(t, directiveSchema)
	schema := codegen.FromSchemaIR(ir)
	q := buildQueryDesc(t, ir, "getUser", "get_user.aql", "select User { id } filter .id = $id<uuid>;")
	err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &typescript.TsGenerator{}, &codegen.Context{OutDir: t.TempDir(), Options: map[string]string{"client": "sqlite"}})
	if err == nil {
		t.Fatal("expected an error for an unknown client option")
	}
	if !strings.Contains(err.Error(), "sqlite") {
		t.Errorf("error should name the bad option, got: %v", err)
	}
}

func TestTsCodegenDirectiveImport(t *testing.T) {
	ir := parseSchema(t, directiveSchema)
	schema := codegen.FromSchemaIR(ir)

	q := buildQueryDesc(t, ir, "", "list_users.aql", "@response UserView\nmulti select User { id, email };")

	dir := t.TempDir()
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &typescript.TsGenerator{}, &codegen.Context{OutDir: dir}); err != nil {
		t.Fatalf("walk: %v", err)
	}

	models := readFile(t, filepath.Join(dir, "models.ts"))
	if !strings.Contains(models, "export interface UserView") {
		t.Errorf("models.ts missing hoisted UserView:\n%s", models)
	}
	query := readFile(t, filepath.Join(dir, "list_users.ts"))
	if !strings.Contains(query, `import type { UserView } from "./models.ts"`) {
		t.Errorf("list_users.ts should import UserView from models.ts:\n%s", query)
	}
	if strings.Contains(query, "export interface UserView") {
		t.Errorf("list_users.ts should not redeclare UserView:\n%s", query)
	}
}
