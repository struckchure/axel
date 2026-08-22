package tests

import (
	"strings"
	"testing"

	axel "github.com/struckchure/axel/core"
	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/compiler"
)

const languageIntegrationSchema = `
enum Status { Draft, Published }
global tenant_id: uuid;

type User {
  required id: uuid;
  required email: str { constraint exclusive; };
}

type Post {
  required id: uuid;
  required tenant_id: uuid;
  required title: str;
  status: Status { default := Status.Draft; };
  link author: User;
  multi reviewers: User;
  policy tenant_only for select using ( .tenant_id = global tenant_id );
}
`

func TestASLAndAQLCompileTogether(t *testing.T) {
	ir := parseSchema(t, languageIntegrationSchema)
	cases := []struct {
		name       string
		query      string
		wantSQL    []string
		wantParams []compiler.ParamInfo
	}{
		{
			name:  "nested select infers enum and scalar parameters",
			query: `multi select Post { id, title, author: { email } } filter .status = $status and .tenant_id = global tenant_id order by .title asc limit $limit<int32>;`,
			wantSQL: []string{
				`FROM "post" p`, `p.status = $1`, `p.tenant_id = current_setting('app.tenant_id', true)::UUID`, `ORDER BY p.title ASC`, `LIMIT $2`,
			},
			wantParams: []compiler.ParamInfo{{Name: "status", AQLType: "str", EnumType: "Status"}, {Name: "limit", AQLType: "int32"}},
		},
		{
			name:  "insert upsert uses exclusive field",
			query: `insert User { email := $email<str> } unless conflict on .email else (update User set { email := $email });`,
			wantSQL: []string{
				`INSERT INTO "user"`, `ON CONFLICT ("email") DO UPDATE SET "email" = $1`, `RETURNING`,
			},
			wantParams: []compiler.ParamInfo{{Name: "email", AQLType: "str"}},
		},
		{
			name:  "update link assignment",
			query: `update Post filter .id = $id<uuid> set { author := (select User filter .email = $email<str>) };`,
			wantSQL: []string{
				`UPDATE "post"`, `author = (SELECT u.id FROM "user" u WHERE u.email = $1::TEXT LIMIT 1)`, `WHERE p.id = $2::UUID`,
			},
			wantParams: []compiler.ParamInfo{{Name: "email", AQLType: "str"}, {Name: "id", AQLType: "uuid"}},
		},
		{
			name:  "delete",
			query: `delete Post filter .title ilike $title<str>;`,
			wantSQL: []string{
				`DELETE FROM "post" p`, `WHERE p.title ilike $1::TEXT`,
			},
			wantParams: []compiler.ParamInfo{{Name: "title", AQLType: "str"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt, err := aql.ParseString(tc.query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			compiled, err := compiler.Compile(stmt, ir)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			gotSQL := normalizeSQL(compiled.SQL)
			for _, want := range tc.wantSQL {
				if !strings.Contains(gotSQL, normalizeSQL(want)) {
					t.Errorf("SQL missing %q:\n%s", want, compiled.SQL)
				}
			}
			if len(compiled.Params) != len(tc.wantParams) {
				t.Fatalf("params = %+v, want %+v", compiled.Params, tc.wantParams)
			}
			for i, want := range tc.wantParams {
				if got := compiled.Params[i]; got.Name != want.Name || got.AQLType != want.AQLType || got.EnumType != want.EnumType {
					t.Errorf("param %d = %+v, want %+v", i, got, want)
				}
			}
		})
	}
}

func TestASLAndAQLCrossLanguageFailures(t *testing.T) {
	ir := parseSchema(t, languageIntegrationSchema)
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"unknown selected field", `select Post { missing };`, "missing"},
		{"unknown global", `select Post { id } filter .tenant_id = global missing;`, "global"},
		{"delta on scalar update", `update Post set { title := { "+": 'bad' } };`, "non-multi"},
		{"limit on singleton select", `select Post { id } limit 2;`, "limit"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt, err := aql.ParseString(tc.query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if _, err := compiler.Compile(stmt, ir); err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Errorf("compile error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestASLPolicyPredicatesCompileNativeAQL(t *testing.T) {
	ir := parseSchema(t, languageIntegrationSchema)
	policies, err := axel.SchemaIRToPolicies(ir)
	if err != nil {
		t.Fatalf("lower policies: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("policies = %+v", policies)
	}
	got := normalizeSQL(policies[0].CreateSQL)
	for _, want := range []string{
		`CREATE POLICY "tenant_only" ON "post" FOR SELECT`,
		`USING (tenant_id = current_setting('app.tenant_id', true)::UUID)`,
	} {
		if !strings.Contains(got, normalizeSQL(want)) {
			t.Errorf("policy SQL missing %q:\n%s", want, policies[0].CreateSQL)
		}
	}
}

func normalizeSQL(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}
