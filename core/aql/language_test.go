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
			name:  "delete",
			query: `delete Post filter .id = $id<uuid>;`,
			check: func(t *testing.T, stmt *Statement) {
				if stmt.Delete == nil || stmt.Delete.TypeName != "Post" || stmt.Delete.Filter == nil {
					t.Errorf("delete = %+v", stmt.Delete)
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
