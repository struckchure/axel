package tests

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
	"github.com/struckchure/axel/generators/typescript"
)

const arrayParamSchema = `
enum UserType { Admin, Runner, Vendor }

type User {
  required id: uuid { constraint pk };
  required email: str;
  required age: int32;
  role: UserType;
}`

// A param declared `multi` binds one array value, so membership against it is
// `= ANY($1::T[])` — Postgres `IN` wants a parenthesised list and would reject
// an array bind.
func TestMultiParamCompilesToAnyArray(t *testing.T) {
	c := compileAQL(t, arrayParamSchema,
		"var ( multi $ids: uuid; )\nmulti select User { id } filter .id in $ids;")

	if !strings.Contains(c.SQL, "u.id = ANY($1::UUID[])") {
		t.Errorf("expected = ANY over an array bind:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "in $1") {
		t.Errorf("array param must not be left as IN:\n%s", c.SQL)
	}
	if len(c.Params) != 1 || !c.Params[0].Multi || c.Params[0].AQLType != "uuid" {
		t.Errorf("expected one multi uuid param, got %+v", c.Params)
	}
}

// The element type comes from the compared column when the declaration omits it.
func TestMultiParamInfersElementType(t *testing.T) {
	c := compileAQL(t, arrayParamSchema,
		"var ( multi $ages; )\nmulti select User { id } filter .age in $ages;")

	if !strings.Contains(c.SQL, "u.age = ANY($1)") {
		t.Errorf("expected = ANY over the bind:\n%s", c.SQL)
	}
	if len(c.Params) != 1 || c.Params[0].AQLType != "int32" {
		t.Errorf("expected the element type inferred as int32, got %+v", c.Params)
	}
}

// An optional array param is guarded like any other optional, and every cast of
// the placeholder must be the array type — casting $1 to both T and T[] makes
// Postgres reject the statement.
func TestOptionalMultiParamCastsConsistently(t *testing.T) {
	c := compileAQL(t, arrayParamSchema,
		"var ( multi $emails: str; )\nmulti select User { id } filter .email in $emails?;")

	if !strings.Contains(c.SQL, "($1::TEXT[] IS NULL OR u.email = ANY($1::TEXT[]))") {
		t.Errorf("expected an array-typed null guard:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "$1::TEXT ") {
		t.Errorf("placeholder cast to the element type as well as the array type:\n%s", c.SQL)
	}
}

// A multi param must reach the generated clients as an array, not a scalar.
func TestMultiParamTypesAsArrayInClients(t *testing.T) {
	ir := parseSchema(t, arrayParamSchema)
	schema := codegen.FromSchemaIR(ir)
	q := buildQueryDesc(t, ir, "usersByIds", "users_by_ids.aql",
		"var ( multi $ids: uuid; multi $roles: UserType; )\nmulti select User { id, email } filter .id in $ids and .role in $roles;")

	if len(q.Params) != 2 || !q.Params[0].IsMulti || !q.Params[1].IsMulti {
		t.Fatalf("expected both params flagged multi, got %+v", q.Params)
	}

	queries := []codegen.QueryDescriptor{q}

	tsDir := t.TempDir()
	if err := codegen.Walk(schema, queries, &typescript.TsGenerator{}, &codegen.Context{OutDir: tsDir}); err != nil {
		t.Fatalf("ts walk: %v", err)
	}
	ts := readFile(t, filepath.Join(tsDir, "users_by_ids.ts"))
	for _, want := range []string{"ids: string[];", "roles: UserType[];"} {
		if !strings.Contains(ts, want) {
			t.Errorf("ts params missing %q:\n%s", want, ts)
		}
	}

	goDir := t.TempDir()
	if err := codegen.Walk(schema, queries, &golang.GoGenerator{}, &codegen.Context{OutDir: goDir}); err != nil {
		t.Fatalf("go walk: %v", err)
	}
	goSrc := readFile(t, filepath.Join(goDir, "users_by_ids.go"))
	// The Go emitter aligns struct fields, so allow the padding between the two.
	for _, want := range []string{`Ids\s+\[\]string`, `Roles\s+\[\]UserType`} {
		if !regexp.MustCompile(want).MatchString(goSrc) {
			t.Errorf("go params missing %q:\n%s", want, goSrc)
		}
	}
}
