package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/compiler"
)

const withSchema = `
type Business {
  required id: uuid;
  required name: str;
}
type ApiKey {
  required id: uuid;
  required label: str;
  required link business: Business;
}
type Transaction {
  required id: uuid;
  required sender_id: uuid;
  required reciever_id: uuid;
  required amount: int32;
  required updated_at: datetime;
}
type User {
  required id: uuid;
  required email: str;
  required active: bool;
  required link business: Business;
}
`

// The motivating query: one business and its api keys bound once, then used at
// three points in the filter. Without a with-block each use site would inline
// (and re-evaluate) its own copy of the subquery.
const withQuery = `
with (
  business := (select Business filter .id = $business_id),
  api_keys := (multi select ApiKey filter .business = $business_id)
)
multi select Transaction
filter (
  business is not null
  and (
    (.sender_id = business.id)
    or (.sender_id in api_keys.id)
    or (.reciever_id in api_keys.id)
  )
)
order by .updated_at desc
limit $limit<int32>?
offset $offset<int32>?;`

func TestWithBlockCompilesToCTEs(t *testing.T) {
	c := compileAQL(t, withSchema, withQuery)

	for _, want := range []string{
		"WITH _with_business AS (",
		"_with_api_keys AS (",
		`FROM "business" b`,
		`FROM "api_key" a`,
	} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("expected %q in:\n%s", want, c.SQL)
		}
	}

	// A plain binding is a single row; a multi binding is a set and must not be
	// capped, or membership would test against one arbitrary key.
	business := cteBody(t, c.SQL, "_with_business")
	if !strings.HasSuffix(strings.TrimSpace(business), "LIMIT 1") {
		t.Errorf("plain binding should end in LIMIT 1, got:\n%s", business)
	}
	if apiKeys := cteBody(t, c.SQL, "_with_api_keys"); strings.Contains(apiKeys, "LIMIT") {
		t.Errorf("multi binding must not be capped with LIMIT, got:\n%s", apiKeys)
	}
}

func TestWithBlockReferenceForms(t *testing.T) {
	c := compileAQL(t, withSchema, withQuery)

	// A bare binding reference means its id, so `business is not null` reads as
	// "the binding matched a row".
	if !strings.Contains(c.SQL, "(SELECT _with_business.id FROM _with_business) IS NOT NULL") {
		t.Errorf("expected a bare binding reference to lower to its id:\n%s", c.SQL)
	}
	// A qualified reference on a plain binding is a scalar subquery.
	if !strings.Contains(c.SQL, "t.sender_id = (SELECT _with_business.id FROM _with_business)") {
		t.Errorf("expected business.id to lower to a scalar subquery:\n%s", c.SQL)
	}
	// A multi binding on the right of `in` is a set.
	for _, want := range []string{
		"t.sender_id IN (SELECT _with_api_keys.id FROM _with_api_keys)",
		"t.reciever_id IN (SELECT _with_api_keys.id FROM _with_api_keys)",
	} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("expected %q in:\n%s", want, c.SQL)
		}
	}
}

// $business_id is written twice, once per binding. The param collector dedupes
// by name, so it must bind once — and the binding params must be numbered ahead
// of the body's, matching the order the CTEs are emitted.
func TestWithBlockParamsBindOnceAndInEmissionOrder(t *testing.T) {
	c := compileAQL(t, withSchema, withQuery)

	var names []string
	for _, p := range c.Params {
		names = append(names, p.Name)
	}
	want := []string{"business_id", "limit", "offset"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("params = %v, want %v", names, want)
	}
	if strings.Count(c.SQL, "$1") != 2 {
		t.Errorf("expected $business_id to bind once and be referenced twice:\n%s", c.SQL)
	}
}

// A param compared against a binding field is typed from that field, not left
// unknown for codegen to guess at.
func TestWithBlockInfersParamTypeFromBindingField(t *testing.T) {
	c := compileAQL(t, withSchema, `
with (business := (select Business filter .id = $business_id))
multi select User filter .email = $email and business.name = $name;`)

	p := paramByName(c, "name")
	if p == nil {
		t.Fatalf("param %q not collected, got %+v", "name", c.Params)
	}
	if p.AQLType != "str" {
		t.Errorf("param name AQLType = %q, want str (from Business.name)", p.AQLType)
	}
}

// A binding shadows a type of the same spelling, and the `_with_` prefix keeps
// its CTE from shadowing that type's table for the rest of the statement.
func TestWithBlockBindingShadowsTypeName(t *testing.T) {
	c := compileAQL(t, withSchema, `
with (Business := (select Business filter .id = $business_id))
multi select User filter .business = Business.id;`)

	if !strings.Contains(c.SQL, "u.business = (SELECT _with_Business.id FROM _with_Business)") {
		t.Errorf("expected the binding to win over the type name:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `FROM "business" b`) {
		t.Errorf("the binding's CTE must not shadow the business table:\n%s", c.SQL)
	}
}

func TestWithBlockOnUpdateAndDelete(t *testing.T) {
	upd := compileAQL(t, withSchema, `
with (business := (select Business filter .id = $business_id))
update User filter .business = business.id set { active := false };`)
	if !strings.HasPrefix(upd.SQL, "WITH _with_business AS (") {
		t.Errorf("update should carry the WITH prefix:\n%s", upd.SQL)
	}

	del := compileAQL(t, withSchema, `
with (business := (select Business filter .id = $business_id))
delete User filter .business = business.id;`)
	if !strings.HasPrefix(del.SQL, "WITH _with_business AS (") {
		t.Errorf("delete should carry the WITH prefix:\n%s", del.SQL)
	}
}

// An insert already emits its own WITH for sub-insert CTEs; bindings share that
// one clause rather than producing a second.
func TestWithBlockOnInsertSharesSingleWithClause(t *testing.T) {
	c := compileAQL(t, withSchema, `
with (business := (select Business filter .id = $business_id))
insert User { email := $email, active := true, business := business.id };`)

	if n := strings.Count(c.SQL, "WITH "); n != 1 {
		t.Errorf("expected exactly one WITH clause, got %d:\n%s", n, c.SQL)
	}
	if !strings.Contains(c.SQL, "(SELECT _with_business.id FROM _with_business)") {
		t.Errorf("expected the binding to be usable in the insert:\n%s", c.SQL)
	}
}

func TestWithBlockErrors(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{{
		name:  "unknown field on a binding",
		query: `with (b := (select Business filter .id = $id)) multi select User filter .email = b.nope;`,
		want:  `has no field "nope"`,
	}, {
		// Lowering this would emit a row-set subquery in a scalar position, which
		// Postgres only rejects at run time.
		name:  "multi binding in a scalar position",
		query: `with (k := (multi select ApiKey filter .business = $id)) multi select User filter .email = k.label;`,
		want:  "use it on the right of `in`",
	}, {
		name:  "shape on a binding — unprojected field reference",
		query: `with (b := (select Business { id } filter .id = $id)) multi select User filter .email = b.name;`,
		want:  `field "name" was not included in the { shape }`,
	}, {
		name:  "shape on a binding — unknown field in shape",
		query: `with (b := (select Business { nope } filter .id = $id)) multi select User filter .email = $e;`,
		want:  `has no field "nope"`,
	}, {
		name:  "shape on a binding — nested sub-shape rejected",
		query: `with (b := (select Business { id: { sub } } filter .id = $id)) multi select User filter .email = $e;`,
		want:  "nested shapes are not supported",
	}, {
		name:  "duplicate binding name",
		query: `with (b := (select Business filter .id = $id), b := (select Business filter .id = $id)) multi select User;`,
		want:  `duplicate binding "b"`,
	}, {
		name:  "binding that is not a sub-select",
		query: `with (b := $id) multi select User filter .email = $e;`,
		want:  "must be a (select ...) or (multi select ...)",
	}, {
		name:  "limit on a plain binding",
		query: `with (b := (select Business filter .id = $id limit 5)) multi select User filter .email = $e;`,
		want:  "require `multi select`",
	}, {
		name:  "unknown type in a binding",
		query: `with (b := (select Nope filter .id = $id)) multi select User filter .email = $e;`,
		want:  `unknown type "Nope"`,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := compileErr(t, withSchema, tc.query)
			if err == nil {
				t.Fatalf("expected an error containing %q, got none", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

// A with-binding may name a { shape } to project only a subset of columns into
// the CTE. This is useful when only one or two fields are needed at use sites,
// keeping the CTE narrow without requiring callers to cast inside the binding.
func TestWithBlockShape(t *testing.T) {
	// Single named field: only id is projected; the CTE body must SELECT only id.
	c := compileAQL(t, withSchema, `
with (
  b := (select Business { id } filter .id = $bid)
)
multi select User filter .business = b.id;`)

	body := cteBody(t, c.SQL, "_with_b")
	if strings.Contains(body, "name") {
		t.Errorf("shaped binding should not project 'name', got CTE body:\n%s", body)
	}
	if !strings.Contains(body, "b.id AS id") && !strings.Contains(body, ".id AS id") {
		t.Errorf("shaped binding should project 'id', got CTE body:\n%s", body)
	}

	// Multi binding with shape + cast: sender_id in api_key.id<str> must work.
	c2 := compileAQL(t, withSchema, `
with (
  api_key := (multi select ApiKey { id } filter .id = $kid<uuid>? or .business = $bid<uuid>?)
)
multi select Transaction
filter .sender_id in api_key.id<str> or .reciever_id in api_key.id<str>;`)

	for _, want := range []string{
		"(SELECT (_with_api_key.id)::TEXT FROM _with_api_key)",
	} {
		if !strings.Contains(c2.SQL, want) {
			t.Errorf("expected %q in:\n%s", want, c2.SQL)
		}
	}

	// A `*` in a shaped binding reverts to full projection (no projectedFields restriction).
	c3 := compileAQL(t, withSchema, `
with (b := (select Business { * } filter .id = $bid))
multi select User filter .business = b.id and b.name = $name;`)

	if !strings.Contains(c3.SQL, "b.name AS name") || !strings.Contains(c3.SQL, "b.id AS id") {
		t.Errorf("star shape should project all columns, got CTE body:\n%s", cteBody(t, c3.SQL, "_with_b"))
	}
}

// A trigger body is spliced into plpgsql, where there is no host statement to
// carry a CTE — so a with-block must be rejected rather than silently dropped.
func TestWithBlockRejectedInTriggerBody(t *testing.T) {
	ir := parseSchema(t, withSchema)
	stmt, err := aql.ParseString(`
with (business := (select Business filter .id = $id))
update User filter .business = business.id set { active := false };`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = compiler.CompileTriggerBody(stmt, ir, ir.ObjectTypes["User"], []string{"id"})
	if err == nil {
		t.Fatal("expected a with-block in a trigger body to be rejected")
	}
	if !strings.Contains(err.Error(), "not supported in a trigger or function body") {
		t.Errorf("error = %q, want it to name the trigger/function restriction", err)
	}
}

// cteBody returns the text of the named CTE, so a test can assert on it without
// matching against the whole statement.
func cteBody(t *testing.T, sql, name string) string {
	t.Helper()
	start := strings.Index(sql, name+" AS (")
	if start < 0 {
		t.Fatalf("CTE %q not found in:\n%s", name, sql)
	}
	rest := sql[start+len(name)+len(" AS ("):]
	depth := 1
	for i, r := range rest {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return rest[:i]
			}
		}
	}
	t.Fatalf("unbalanced parens after CTE %q in:\n%s", name, sql)
	return ""
}
