package aql

import (
	"strings"
	"testing"
)

func TestParseLanguageStatements(t *testing.T) {
	cases := []struct {
		name  string
		query string
		check func(*testing.T, *Statement)
	}{
		{
			name: "select with directives vars and nested shape",
			query: `@name ListPosts
@rel_load_strategy join
var (
  $status<Status>?;
  $limit<int32>;
)
with (authors := (multi select User { id } filter .active = true);)
multi select Post { id, title, author: { id, email }, total := count(.id) filter .status = $status? } filter .author.id in authors.id order by .title desc limit $limit offset 2;`,
			check: func(t *testing.T, stmt *Statement) {
				if got := stmt.DirectiveMap(); got["name"] != "ListPosts" || got["rel_load_strategy"] != "join" {
					t.Errorf("directives = %v", got)
				}
				if len(stmt.Vars) != 1 || len(stmt.Vars[0].Params) != 2 || !stmt.Vars[0].Params[0].Optional {
					t.Errorf("vars = %+v", stmt.Vars)
				}
				if stmt.With == nil || len(stmt.With.Bindings) != 1 || stmt.With.Bindings[0].Name != "authors" {
					t.Errorf("with = %+v", stmt.With)
				}
				if stmt.Select == nil || !stmt.Select.Multi || stmt.Select.Body.Shape == nil || len(stmt.Select.Body.Shape.Fields) != 4 {
					t.Fatalf("select = %+v", stmt.Select)
				}
				if nested := stmt.Select.Body.Shape.Fields[2]; nested.Name != "author" || nested.SubShape == nil || len(nested.SubShape.Fields) != 2 {
					t.Errorf("nested shape = %+v", nested)
				}
				if agg := stmt.Select.Body.Shape.Fields[3]; agg.AggFilter == nil || agg.Computed == nil {
					t.Errorf("aggregate field = %+v", agg)
				}
			},
		},
		{
			name:  "insert upsert",
			query: `insert User { email := $email<str>, active := true } unless conflict on .email else (update User set { active := false });`,
			check: func(t *testing.T, stmt *Statement) {
				if stmt.Insert == nil || len(stmt.Insert.Assignments) != 2 || stmt.Insert.Conflict == nil || stmt.Insert.Conflict.Else == nil {
					t.Errorf("insert = %+v", stmt.Insert)
				}
			},
		},
		{
			name:  "update cast and null test",
			query: `update Post filter .published_at is not null and .author.id = $author<uuid> set { title := .title<str> };`,
			check: func(t *testing.T, stmt *Statement) {
				if stmt.Update == nil || stmt.Update.Filter == nil || len(stmt.Update.Assignments) != 1 {
					t.Errorf("update = %+v", stmt.Update)
				}
			},
		},
		{
			name:  "update multi link delta",
			query: `update Organization filter .id = $id<uuid> set { name := $name, members := { "+": (multi select User filter .email in $emails), "-": (select User filter .id = $old_id) } };`,
			check: func(t *testing.T, stmt *Statement) {
				if stmt.Update == nil || len(stmt.Update.Assignments) != 2 {
					t.Fatalf("expected 2 assignments, got %+v", stmt.Update)
				}
				delta := stmt.Update.Assignments[1].LinkDelta
				if delta == nil || len(delta.Items) != 2 {
					t.Fatalf("expected LinkDelta with 2 items, got %+v", delta)
				}
				if delta.Items[0].NormalizedOp() != "+" || delta.Items[1].NormalizedOp() != "-" {
					t.Errorf("expected ops + and -, got %s and %s", delta.Items[0].NormalizedOp(), delta.Items[1].NormalizedOp())
				}
			},
		},
		{
			name:  "delete",
			query: `delete Post filter .id = $id<uuid>;`,
			check: func(t *testing.T, stmt *Statement) {
				if stmt.Delete == nil || stmt.Delete.TypeName != "Post" || stmt.Delete.Filter == nil {
					t.Errorf("delete = %+v", stmt.Delete)
				}
			},
		},
		{
			name:  "select with arithmetic in shape and filter",
			query: `select Order { id, subtotal, total := .subtotal * 1.2 + 5, discount := - .discount } filter .quantity * .unit_price >= 100 - $rebate;`,
			check: func(t *testing.T, stmt *Statement) {
				if stmt.Select == nil || stmt.Select.Body.Shape == nil || len(stmt.Select.Body.Shape.Fields) != 4 {
					t.Fatalf("select = %+v", stmt.Select)
				}
				totalField := stmt.Select.Body.Shape.Fields[2]
				if totalField.Name != "total" || totalField.Computed == nil {
					t.Fatalf("total field = %+v", totalField)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt, err := ParseString(tc.query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			tc.check(t, stmt)
			printed := Print(stmt)
			roundTrip, err := ParseString(printed)
			if err != nil {
				t.Fatalf("re-parse printed query: %v\n%s", err, printed)
			}
			if roundTrip == nil {
				t.Fatal("printed query parsed to nil statement")
			}
		})
	}
}

func TestParseExpressionsAndRejectsMalformedInput(t *testing.T) {
	for _, expr := range []string{
		`.expires_at is null or (.active = true and .score >= 1.5)`,
		`global tenant_id = $tenant<uuid>`,
		`(select User { id } filter .email ilike $email?).id<uuid>`,
		`.price * .quantity + $tax`,
		`(.price + $fee) * (1.0 - .discount / 100.0)`,
		`- .offset + 10`,
		`.a / .b - .c * .d`,
		`- ($a + $b)`,
	} {
		if _, err := ParseExpr(expr); err != nil {
			t.Errorf("ParseExpr(%q): %v", expr, err)
		}
	}

	for _, query := range []string{
		`select User { id `,
		`insert User { email = $email };`,
		`update User set { email := };`,
		`delete filter .id = $id;`,
	} {
		if _, err := ParseString(query); err == nil {
			t.Errorf("ParseString(%q) succeeded, want error", query)
		}
	}

	if _, err := ParseExpr(`.id = `); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Errorf("malformed expression error = %v", err)
	}
}

func TestArithmeticPrecedence(t *testing.T) {
	expr, err := ParseExpr(`.a + .b * .c`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// .a + (.b * .c): AddExpr has 2 terms (Left: .a, Rest[0]: + (.b * .c))
	cmp := expr.SingleCmp()
	if cmp == nil || cmp.Left == nil || len(cmp.Left.Rest) != 1 {
		t.Fatalf("expected AddExpr with 1 op in Rest, got %+v", cmp)
	}
	if cmp.Left.Rest[0].Op != "+" {
		t.Errorf("expected '+' op, got %s", cmp.Left.Rest[0].Op)
	}
	mul := cmp.Left.Rest[0].Right
	if mul == nil || len(mul.Rest) != 1 || mul.Rest[0].Op != "*" {
		t.Errorf("expected MulExpr with '*' op, got %+v", mul)
	}

	// Formatted roundtrip
	formatted, err := Format([]byte(`select Item { val := .a + .b * .c / .d - .e };`))
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if !strings.Contains(formatted, ".a + .b * .c / .d - .e") {
		t.Errorf("unexpected formatted output: %s", formatted)
	}
}
