package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
)

const boolSchema = `
type User { required id: uuid; required email: str; }
type Project {
  required id: uuid;
  required name: str;
  required organization: str;
  required active: bool;
  age: int32;
  created_at: datetime;
  required link owner: User;
  multi link members: User;
}
`

// A filter may chain comparisons with `and` — the case that motivated boolean
// expressions: a flat one-binop grammar rejected the second condition outright.
func TestFilterChainsAnd(t *testing.T) {
	c := compileAQL(t, boolSchema, `
multi select Project { *, members: { id } }
filter .owner = $owner<str> and .organization = $application<str>
order by .created_at desc
limit $limit<int32>?
offset $offset<int32>?;`)

	if !strings.Contains(c.SQL, `p.owner = $1 AND p.organization = $2`) {
		t.Errorf("expected a single AND-joined WHERE clause:\n%s", c.SQL)
	}
	want := []string{"owner", "application", "limit", "offset"}
	if len(c.Params) != len(want) {
		t.Fatalf("expected %d params, got %d: %+v", len(want), len(c.Params), c.Params)
	}
	for i, name := range want {
		if c.Params[i].Name != name {
			t.Errorf("param $%d = %q, want %q", i+1, c.Params[i].Name, name)
		}
	}
}

// `and` chains to any length, and `or` chains too.
func TestFilterChainsAndOrToAnyLength(t *testing.T) {
	c := compileAQL(t, boolSchema, `multi select Project filter .name = $a<str> and .organization = $b<str> and .active = true;`)
	if !strings.Contains(c.SQL, `p.name = $1 AND p.organization = $2 AND p.active = true`) {
		t.Errorf("three-way and chain:\n%s", c.SQL)
	}

	c = compileAQL(t, boolSchema, `multi select Project filter .name = $a<str> or .organization = $b<str> or .active = true;`)
	if !strings.Contains(c.SQL, `p.name = $1 OR p.organization = $2 OR p.active = true`) {
		t.Errorf("three-way or chain:\n%s", c.SQL)
	}
}

// `and` binds tighter than `or`, so `a or b and c` means `a or (b and c)`. The
// emitted SQL makes the grouping explicit rather than leaning on SQL precedence.
func TestFilterAndBindsTighterThanOr(t *testing.T) {
	c := compileAQL(t, boolSchema, `multi select Project filter .name = $a<str> or .organization = $b<str> and .active = true;`)
	if !strings.Contains(c.SQL, `p.name = $1 OR (p.organization = $2 AND p.active = true)`) {
		t.Errorf("and should bind tighter than or:\n%s", c.SQL)
	}
}

// Parenthesized groups override precedence and survive into the SQL.
func TestFilterParenthesizedGroups(t *testing.T) {
	c := compileAQL(t, boolSchema, `
multi select Project
filter (.name = $a<str> or .organization = $b<str>)
   and (.active = true or .age = $c<int32>)
   and .owner = $owner<str>;`)

	want := `(p.name = $1 OR p.organization = $2) AND (p.active = true OR p.age = $3) AND p.owner = $4`
	if !strings.Contains(c.SQL, want) {
		t.Errorf("grouped filter should emit %q:\n%s", want, c.SQL)
	}
	for i, name := range []string{"a", "b", "c", "owner"} {
		if c.Params[i].Name != name {
			t.Errorf("param $%d = %q, want %q", i+1, c.Params[i].Name, name)
		}
	}
}

// Groups nest to any depth — the inner grouping is not flattened away.
func TestFilterNestedGroups(t *testing.T) {
	c := compileAQL(t, boolSchema, `multi select Project filter ((.name = $a<str> or .organization = $b<str>) and .active = true) or .age = $c<int32>;`)

	want := `((p.name = $1 OR p.organization = $2) AND p.active = true) OR p.age = $3`
	if !strings.Contains(c.SQL, want) {
		t.Errorf("nested group should emit %q:\n%s", want, c.SQL)
	}
}

// An optional param relaxes only its own comparison. If the null guard were
// applied to the whole conjunction, omitting $owner would also let rows through
// that fail the .name test.
func TestOptionalParamGuardsOnlyItsOwnComparison(t *testing.T) {
	c := compileAQL(t, boolSchema, `multi select Project filter .owner = $owner<str>? and .name = $name<str>;`)

	want := `($1::UUID IS NULL OR p.owner = $1) AND p.name = $2`
	if !strings.Contains(c.SQL, want) {
		t.Errorf("optional param must guard only its own comparison, want %q:\n%s", want, c.SQL)
	}
	if strings.Contains(c.SQL, `IS NULL OR p.owner = $1 AND`) {
		t.Errorf("null guard leaked across the conjunction:\n%s", c.SQL)
	}
}

// Boolean expressions round-trip through the printer.
func TestBoolFilterRoundTrips(t *testing.T) {
	src := `multi select Project { id } filter (.name = $a or .active = true) and .organization = $b;`
	stmt, err := aql.ParseString(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := aql.Print(stmt)
	for _, want := range []string{"(.name = $a or .active = true)", "and .organization = $b"} {
		if !strings.Contains(got, want) {
			t.Errorf("printed AQL missing %q:\n%s", want, got)
		}
	}
}

// Single-comparison filters, coalesce, and the clauses around a filter are
// unaffected by the boolean-expression restructure.
func TestSingleComparisonFilterUnchanged(t *testing.T) {
	c := compileAQL(t, boolSchema, `multi select Project filter .name = $name<str> order by .created_at desc limit $limit<int32>;`)
	if !strings.Contains(c.SQL, `WHERE p.name = $1`) {
		t.Errorf("single comparison should compile to a bare WHERE:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, " AND ") || strings.Contains(c.SQL, " OR ") {
		t.Errorf("single comparison should emit no boolean connector:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "ORDER BY") || !strings.Contains(c.SQL, "LIMIT") {
		t.Errorf("clauses after the filter should still compile:\n%s", c.SQL)
	}
}
