package tests

import (
	"strings"
	"testing"
)

// Mutations must compile to a SINGLE SQL command — no embedded BEGIN;/COMMIT; —
// otherwise the parameterized (extended) protocol rejects them with
// "cannot insert multiple commands into a prepared statement" (42601).
func TestMutationsAreSingleStatement(t *testing.T) {
	schema := `type User { required id: uuid; required email: str; name: str; }`

	cases := map[string]string{
		"insert": `insert User { email := $email, name := $name };`,
		"update": `update User filter .id = $id set { name := $name };`,
		"delete": `delete User filter .id = $id;`,
	}

	for op, q := range cases {
		c := compileAQL(t, schema, q)
		if strings.Contains(c.SQL, "BEGIN") || strings.Contains(c.SQL, "COMMIT") {
			t.Errorf("%s SQL should not wrap BEGIN/COMMIT:\n%s", op, c.SQL)
		}
		// A single trailing ';' → exactly one command.
		if n := strings.Count(strings.TrimRight(c.SQL, "\n"), ";"); n != 1 {
			t.Errorf("%s SQL should be a single statement (found %d ';'):\n%s", op, n, c.SQL)
		}
	}
}

// A bare optional param in a `set` clause is plain nullable: passing nil writes
// SQL NULL to the column. No COALESCE — keeping the current value is opt-in via `??`.
func TestOptionalAssignmentWritesNull(t *testing.T) {
	schema := `type User { required id: uuid; email: str; }`
	c := compileAQL(t, schema, `update User filter .id = $id set { email := $email? };`)
	if !strings.Contains(c.SQL, "email = $1") {
		t.Errorf("expected bare optional assignment to write the param directly, got:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "COALESCE") {
		t.Errorf("bare optional assignment must not coalesce:\n%s", c.SQL)
	}
}

func TestRequiredAssignmentNotCoalesced(t *testing.T) {
	schema := `type User { required id: uuid; email: str; }`
	c := compileAQL(t, schema, `update User filter .id = $id set { email := $email };`)
	if strings.Contains(c.SQL, "COALESCE") {
		t.Errorf("required assignment should overwrite unconditionally:\n%s", c.SQL)
	}
}

// `$param ?? .field` keeps the current value when the param is null, compiling to
// COALESCE with the param cast to the column's SQL type.
func TestCoalesceAssignmentKeepsCurrent(t *testing.T) {
	schema := `type User { required id: uuid; email: str; }`
	c := compileAQL(t, schema, `update User filter .id = $id set { email := $email? ?? .email };`)
	if !strings.Contains(c.SQL, "email = COALESCE($1::TEXT, u.email)") {
		t.Errorf("expected coalesce-to-current-value, got:\n%s", c.SQL)
	}
	if len(c.Params) == 0 || c.Params[0].Name != "email" || !c.Params[0].Optional {
		t.Errorf("expected an optional 'email' param, got %+v", c.Params)
	}
}

// Enum-backed and json columns resolve to TEXT and JSONB respectively; the `??`
// cast must follow the column's SQL type, not the AQL type name.
func TestCoalesceAssignmentCastsEnumAndJSON(t *testing.T) {
	schema := `
enum ApplicationOs { Linux, Darwin }
type Application {
  required id: uuid;
  os: ApplicationOs;
  build_system_config: json;
}`
	c := compileAQL(t, schema, `update Application filter .id = $id set {
      os := $os<ApplicationOs>? ?? .os,
      build_system_config := $cfg<json>? ?? .build_system_config
    };`)

	for _, want := range []string{
		"os = COALESCE($1::TEXT, a.os)",
		"build_system_config = COALESCE($2::JSONB, a.build_system_config)",
	} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("expected %q in:\n%s", want, c.SQL)
		}
	}
}
