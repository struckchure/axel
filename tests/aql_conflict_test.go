package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/compiler"
)

// A schema with a field-level exclusive constraint on .email.
const conflictSchema = `type User {
  required id: uuid;
  required email: str { constraint exclusive; };
  name: str;
}`

// A schema with a composite exclusive constraint on (.email, .tenant_id).
const compositeConflictSchema = `type User {
  required id: uuid;
  required email: str;
  required tenant_id: uuid;
  name: str;
  constraint exclusive on (.email, .tenant_id);
}`

func TestConflictDoNothingBare(t *testing.T) {
	c := compileAQL(t, conflictSchema, `insert User { email := $email, name := $name } unless conflict;`)
	if !strings.Contains(c.SQL, "ON CONFLICT DO NOTHING") {
		t.Errorf("expected bare ON CONFLICT DO NOTHING, got:\n%s", c.SQL)
	}
	assertSingleStatement(t, c.SQL)
}

func TestConflictDoNothingOnField(t *testing.T) {
	c := compileAQL(t, conflictSchema, `insert User { email := $email, name := $name } unless conflict on .email;`)
	if !strings.Contains(c.SQL, `ON CONFLICT ("email") DO NOTHING`) {
		t.Errorf("expected ON CONFLICT (\"email\") DO NOTHING, got:\n%s", c.SQL)
	}
	assertSingleStatement(t, c.SQL)
}

func TestConflictDoNothingComposite(t *testing.T) {
	c := compileAQL(t, compositeConflictSchema,
		`insert User { email := $email, tenant_id := $tenant_id, name := $name } unless conflict on (.email, .tenant_id);`)
	if !strings.Contains(c.SQL, `ON CONFLICT ("email", "tenant_id") DO NOTHING`) {
		t.Errorf("expected composite ON CONFLICT, got:\n%s", c.SQL)
	}
	assertSingleStatement(t, c.SQL)
}

func TestConflictUpsert(t *testing.T) {
	c := compileAQL(t, conflictSchema,
		`insert User { email := $email, name := $name } unless conflict on .email else (update User set { name := $name });`)
	if !strings.Contains(c.SQL, `ON CONFLICT ("email") DO UPDATE SET "name" =`) {
		t.Errorf("expected ON CONFLICT (\"email\") DO UPDATE SET, got:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "RETURNING") {
		t.Errorf("upsert should still RETURNING, got:\n%s", c.SQL)
	}
	assertSingleStatement(t, c.SQL)
}

func TestConflictNonExclusiveTargetErrors(t *testing.T) {
	assertConflictError(t, conflictSchema,
		`insert User { email := $email, name := $name } unless conflict on .name;`,
		"exclusive")
}

func TestConflictElseWithoutTargetErrors(t *testing.T) {
	// `else` without an `on` target is invalid (Postgres DO UPDATE needs a target).
	assertConflictError(t, conflictSchema,
		`insert User { email := $email } unless conflict else (update User set { name := $name });`,
		"requires an `on` target")
}

func TestConflictElseTypeMismatchErrors(t *testing.T) {
	schema := conflictSchema + "\ntype Org { required id: uuid; name: str; }"
	assertConflictError(t, schema,
		`insert User { email := $email } unless conflict on .email else (update Org set { name := $name });`,
		"must match")
}

// assertSingleStatement enforces the mutation single-statement invariant.
func assertSingleStatement(t *testing.T, sql string) {
	t.Helper()
	if strings.Contains(sql, "BEGIN") || strings.Contains(sql, "COMMIT") {
		t.Errorf("SQL should not wrap BEGIN/COMMIT:\n%s", sql)
	}
	if n := strings.Count(strings.TrimRight(sql, "\n"), ";"); n != 1 {
		t.Errorf("SQL should be a single statement (found %d ';'):\n%s", n, sql)
	}
}

// assertConflictError compiles a query expected to fail, checking the message.
func assertConflictError(t *testing.T, schema, query, want string) {
	t.Helper()
	ir := parseSchema(t, schema)
	stmt, err := aql.ParseString(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	_, err = compiler.Compile(stmt, ir)
	if err == nil {
		t.Fatalf("expected compile error containing %q, got none", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("expected error containing %q, got: %v", want, err)
	}
}
