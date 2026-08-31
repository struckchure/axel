package tests

import (
	"strings"
	"testing"
)

const warningSchema = `
enum UserType { Admin, Runner }

type User {
  required id: uuid { constraint pk };
  required email: str;
  multi roles: UserType;
}`

// A function call the compiler doesn't recognise still compiles — passing SQL
// through is the escape hatch for the long tail of Postgres functions — but it
// is reported, because `distinct(...)` compiles cleanly and then fails at the
// database (DISTINCT is a keyword, not a function).
func TestUnknownFunctionWarns(t *testing.T) {
	c := compileAQL(t, warningSchema, `multi select User { id, r := distinct(.roles) };`)
	if len(c.Warnings) == 0 {
		t.Fatalf("expected a warning for distinct(...), got none")
	}
	joined := strings.Join(c.Warnings, "\n")
	if !strings.Contains(joined, "SQL keyword") || !strings.Contains(joined, "distinct") {
		t.Errorf("warning should name distinct as a keyword, got: %s", joined)
	}
	// Still compiles — the warning must not block the query.
	if !strings.Contains(c.SQL, "distinct(u.roles)") {
		t.Errorf("query should still compile through, got:\n%s", c.SQL)
	}
}

func TestUnknownFunctionNameWarns(t *testing.T) {
	c := compileAQL(t, warningSchema, `multi select User { id, e := lowre(.email) };`)
	joined := strings.Join(c.Warnings, "\n")
	if !strings.Contains(joined, "unknown function") || !strings.Contains(joined, "lowre") {
		t.Errorf("expected an unknown-function warning naming lowre, got: %v", c.Warnings)
	}
}

// Known builtins, aggregates, and schema-declared functions are silent.
func TestKnownFunctionsDoNotWarn(t *testing.T) {
	for _, q := range []string{
		`multi select User { id, e := lower(.email) };`,
		`multi select User { id, n := array_position(.roles, UserType.Admin) };`,
		`select User { c := count() };`,
		`multi select User { id, t := coalesce(.email, 'none') };`,
		`multi select User { id, d := date_trunc('day', .id) };`,
	} {
		if w := compileAQL(t, warningSchema, q).Warnings; len(w) != 0 {
			t.Errorf("%s\n  unexpected warnings: %v", q, w)
		}
	}
}

func TestSchemaDeclaredFunctionDoesNotWarn(t *testing.T) {
	schema := `
@language plpgsql
function shout(value: text) -> text { return upper(value); };

type User {
  required id: uuid { constraint pk };
  required email: str;
}`
	if w := compileAQL(t, schema, `multi select User { id, e := shout(.email) };`).Warnings; len(w) != 0 {
		t.Errorf("schema-declared function should not warn, got: %v", w)
	}
}

// The old message said "non-multi field", which reads as a contradiction when
// the field really is declared `multi` — it is just not a link.
func TestDeltaAssignmentErrorNamesTheActualProblem(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"multi scalar", `update User set { roles := { "+": UserType.Admin } };`, `"roles" is a multi scalar`},
		{"plain scalar", `update User set { email := { "+": 'x' } };`, `"email" is a scalar`},
		{"unknown field", `update User set { nope := { "+": 'x' } };`, `no multi link`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := compileErr(t, warningSchema, tc.query)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), "requires a multi link") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
