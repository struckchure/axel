package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/core/compiler"
	"github.com/struckchure/axel/generators/golang"
)

const paramSchema = `type User { required id: uuid; required email: str; required age: int32; }`

func compileAQL(t *testing.T, schema, query string) *compiler.CompiledSQL {
	t.Helper()
	ir := parseSchema(t, schema)
	stmt, err := aql.ParseString(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	c, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	return c
}

func buildQueryDesc(t *testing.T, ir *asl.SchemaIR, name, file, query string) codegen.QueryDescriptor {
	t.Helper()
	stmt, err := aql.ParseString(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	compiled, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	desc, err := codegen.BuildQueryDescriptor(name, file, stmt, compiled, ir)
	if err != nil {
		t.Fatalf("descriptor %q: %v", query, err)
	}
	return desc
}

func TestOptionalParamSkipsWhenNull(t *testing.T) {
	c := compileAQL(t, paramSchema, `select User { id } filter .email = $email?;`)
	if !strings.Contains(c.SQL, "($1 IS NULL OR u.email = $1)") {
		t.Errorf("expected skip-when-null wrap, got:\n%s", c.SQL)
	}
	if len(c.Params) != 1 || c.Params[0].Name != "email" || !c.Params[0].Optional {
		t.Errorf("expected one optional param 'email', got %+v", c.Params)
	}
}

func TestRequiredParamNotWrapped(t *testing.T) {
	c := compileAQL(t, paramSchema, `select User { id } filter .email = $email;`)
	if strings.Contains(c.SQL, "IS NULL OR") {
		t.Errorf("required param should not be wrapped:\n%s", c.SQL)
	}
	if c.Params[0].Optional {
		t.Errorf("param should not be optional")
	}
}

func TestSingleSelectLimitOne(t *testing.T) {
	c := compileAQL(t, paramSchema, `select User { id };`)
	if !strings.Contains(c.SQL, "LIMIT 1") {
		t.Errorf("single select missing LIMIT 1:\n%s", c.SQL)
	}
}

func TestMultiSelectNoImplicitLimit(t *testing.T) {
	c := compileAQL(t, paramSchema, `multi select User { id };`)
	if strings.Contains(c.SQL, "LIMIT") {
		t.Errorf("multi select should have no implicit LIMIT:\n%s", c.SQL)
	}
}

func TestMultiSelectHonoursExplicitLimit(t *testing.T) {
	c := compileAQL(t, paramSchema, `multi select User { id } limit 5;`)
	if !strings.Contains(c.SQL, "LIMIT 5") {
		t.Errorf("multi select should honour explicit limit:\n%s", c.SQL)
	}
}

func TestLimitWithoutMultiErrors(t *testing.T) {
	ir := parseSchema(t, paramSchema)
	stmt, err := aql.ParseString(`select User { id } limit 5;`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := compiler.Compile(stmt, ir); err == nil {
		t.Fatal("expected error for limit without multi")
	}
}

func TestDescriptorParamOptionalAndMultiple(t *testing.T) {
	ir := parseSchema(t, paramSchema)

	single := buildQueryDesc(t, ir, "getUser", "get_user.aql", `select User { id, email } filter .email = $email?;`)
	if single.Result.IsMultiple {
		t.Errorf("plain select should be single (IsMultiple=false)")
	}
	if len(single.Params) != 1 || !single.Params[0].IsOptional {
		t.Errorf("expected optional param descriptor, got %+v", single.Params)
	}

	multi := buildQueryDesc(t, ir, "listUser", "list_user.aql", `multi select User { id, email };`)
	if !multi.Result.IsMultiple {
		t.Errorf("multi select should be IsMultiple=true")
	}
}

func TestGoCodegenOptionalParamAndRowShape(t *testing.T) {
	ir := parseSchema(t, paramSchema)
	schema := codegen.FromSchemaIR(ir)

	qSingle := buildQueryDesc(t, ir, "getUser", "get_user.aql", `select User { id, email } filter .email = $email?;`)
	qMulti := buildQueryDesc(t, ir, "listUser", "list_user.aql", `multi select User { id, email };`)

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{qSingle, qMulti}, &golang.GoGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	single := readFile(t, filepath.Join(dir, "get_user.go"))
	if !strings.Contains(single, "Email *string") {
		t.Errorf("optional param should be *string:\n%s", single)
	}
	if !strings.Contains(single, "(*GetUserRow, error)") {
		t.Errorf("single select should return *Row:\n%s", single)
	}

	multi := readFile(t, filepath.Join(dir, "list_user.go"))
	if !strings.Contains(multi, "([]ListUserRow, error)") {
		t.Errorf("multi select should return []Row:\n%s", multi)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
