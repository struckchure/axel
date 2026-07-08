package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
)

const relSchema = `
type User { required id: uuid; required email: str; }
type Project {
  required id: uuid;
  required name: str;
  required link owner: User;
  multi link members: User;
}
`

// A multi-link must compile to a correlated json_agg subquery that joins through
// the junction table using columns named after the referenced tables (user /
// project), with no dangling LATERAL.
func TestMultiLinkJoinsJunctionTable(t *testing.T) {
	c := compileAQL(t, relSchema, `select Project { id, members: { id } };`)

	for _, want := range []string{
		`JOIN "user" u_members ON u_members.id = jt_members.user`,
		`WHERE jt_members.project = p.id`,
	} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("multi-link SQL missing %q:\n%s", want, c.SQL)
		}
	}
	for _, bad := range []string{"user_id", "p_id", "LATERAL"} {
		if strings.Contains(c.SQL, bad) {
			t.Errorf("multi-link SQL should not contain %q:\n%s", bad, c.SQL)
		}
	}
}

// `*` inside a shape expands to all scalar properties plus single-link FK
// columns, and can be combined with explicit nested selections.
func TestShapeSplatExpandsScalarsAndLinks(t *testing.T) {
	// Round-trips through the parser/printer with the `*`.
	stmt, err := aql.ParseString(`multi select Project { *, members: { id } };`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := aql.Print(stmt); !strings.Contains(got, "*") {
		t.Errorf("printed shape should retain splat:\n%s", got)
	}

	c := compileAQL(t, relSchema, `multi select Project { *, members: { id } };`)

	// Splat pulls in scalars + the single-link FK (owner).
	for _, want := range []string{"p.id AS id", "p.name AS name", "p.owner AS owner"} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("splat SQL missing %q:\n%s", want, c.SQL)
		}
	}
	// The explicit multi-link is included exactly once (as the json_agg), and the
	// splat did not also emit a bare `members` column.
	if n := strings.Count(c.SQL, "AS members"); n != 1 {
		t.Errorf("expected exactly one members column, got %d:\n%s", n, c.SQL)
	}
	// owner must not be duplicated by the splat.
	if n := strings.Count(c.SQL, "AS owner"); n != 1 {
		t.Errorf("expected exactly one owner column, got %d:\n%s", n, c.SQL)
	}
}

// An explicitly-listed field wins over the splat (no duplicate column).
func TestShapeSplatExplicitFieldWins(t *testing.T) {
	c := compileAQL(t, relSchema, `select Project { *, owner: { email } };`)
	// owner is listed explicitly as a nested object, so the splat must not also
	// emit the flat owner FK column.
	if n := strings.Count(c.SQL, "AS owner"); n != 1 {
		t.Errorf("expected exactly one owner column, got %d:\n%s", n, c.SQL)
	}
	if !strings.Contains(c.SQL, "u_owner") {
		t.Errorf("explicit owner should compile to a nested object subquery:\n%s", c.SQL)
	}
}

// An optional filter param casts its IS NULL check to the compared column's SQL
// type so Postgres can determine the param type when the value is null (42P08).
func TestOptionalFilterParamCastsToColumnType(t *testing.T) {
	// Link filter → cast to the FK column type (UUID), regardless of a mismatched
	// annotation, so the cast agrees with `a.owner = $1`.
	c := compileAQL(t, relSchema, `multi select Project filter .owner = $owner<str>?;`)
	if !strings.Contains(c.SQL, "($1::UUID IS NULL OR") {
		t.Errorf("link optional filter should cast the param to UUID:\n%s", c.SQL)
	}

	// Scalar filter → cast to the property's own SQL type.
	c = compileAQL(t, relSchema, `multi select Project filter .name = $name<str>?;`)
	if !strings.Contains(c.SQL, "($1::TEXT IS NULL OR") {
		t.Errorf("scalar optional filter should cast the param to TEXT:\n%s", c.SQL)
	}
}
