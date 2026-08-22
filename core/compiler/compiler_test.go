package compiler

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

const compilerCoverageSchema = `
global tenant_id: uuid;
enum Status { Draft, Published }
type User { required id: uuid; required email: str { constraint exclusive; }; }
type Post {
  required id: uuid;
  required tenant_id: uuid;
  title: str;
  status: Status;
  link author: User;
  multi reviewers: User;
}`

func compilerSchema(t *testing.T) *asl.SchemaIR {
	t.Helper()
	src, err := asl.Parse([]byte(compilerCoverageSchema))
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	ir, err := (&asl.Resolver{}).Resolve(src)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}
	return ir
}

func TestCompileCoreStatements(t *testing.T) {
	ir := compilerSchema(t)
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"select links and globals", `multi select Post { *, author: { email }, reviewers: { id } } filter .tenant_id = global tenant_id order by .title desc limit $limit<int32>;`, []string{`FROM "post" p`, `current_setting('app.tenant_id', true)::UUID`, `ORDER BY p.title DESC`, `LIMIT $1`, `json_agg`}},
		{"aggregate group by", `multi select Post { status, total := count(.id) filter .title is not null } group by .status having count(.id) > 0;`, []string{`GROUP BY p.status`, `COUNT(p.id) FILTER (WHERE p.title IS NOT NULL)`, `HAVING COUNT(p.id) > 0`}},
		{"insert conflict", `insert User { email := $email<str> } unless conflict on .email;`, []string{`INSERT INTO "user"`, `ON CONFLICT ("email") DO NOTHING`, `RETURNING`}},
		{"update link", `update Post filter .id = $id<uuid> set { author := (select User filter .email = $email<str>) };`, []string{`UPDATE "post"`, `author = (SELECT u.id FROM "user" u WHERE u.email = $1::TEXT LIMIT 1)`, `WHERE p.id = $2::UUID`}},
		{"delete", `delete Post filter .status = Published;`, []string{`DELETE FROM "post" p`, `WHERE p.status = Published`}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt, err := aql.ParseString(tc.query)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}
			compiled, err := Compile(stmt, ir)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(compiled.SQL, want) {
					t.Errorf("SQL missing %q:\n%s", want, compiled.SQL)
				}
			}
		})
	}
}

func TestCompilerPublicHelpers(t *testing.T) {
	ir := compilerSchema(t)

	stmt, err := aql.ParseString(`multi select Post { author: { email } };`)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := CompileWithOptions(stmt, ir, CompileOptions{RelLoadStrategy: "join"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(joined.SQL, "LEFT JOIN LATERAL") {
		t.Errorf("join strategy SQL:\n%s", joined.SQL)
	}

	inline, err := CompileInline(`delete Post filter .title = 'old';`, ir)
	if err != nil || inline != `'DELETE FROM "post" p WHERE p.title = ''old'';'` {
		t.Errorf("inline = %q, %v", inline, err)
	}
	if _, err := CompileInline(`delete Post filter .id = $id<uuid>;`, ir); err == nil || !strings.Contains(err.Error(), "parameters") {
		t.Errorf("parameterized inline error = %v", err)
	}

	expr, err := aql.ParseExpr(`.tenant_id = global tenant_id`)
	if err != nil {
		t.Fatal(err)
	}
	predicate, err := CompilePolicyPredicate(expr, ir.ObjectTypes["Post"], ir)
	if err != nil || !strings.Contains(predicate, `current_setting('app.tenant_id', true)::UUID`) {
		t.Errorf("policy predicate = %q, %v", predicate, err)
	}

	trigger, err := aql.ParseString(`insert User { email := __new__.title }`)
	if err != nil {
		t.Fatal(err)
	}
	body, err := CompileTriggerBody(trigger, ir, ir.ObjectTypes["Post"], nil)
	if err != nil || strings.Contains(body, "RETURNING") || !strings.Contains(body, `NEW."title"`) {
		t.Errorf("trigger body = %q, %v", body, err)
	}

	compiled := &CompiledSQL{SQL: "SELECT $1;", Params: []ParamInfo{{Name: "id", AQLType: "uuid"}}}
	if got, want := compiled.Full(), "-- $1: id (uuid)\nSELECT $1;"; got != want {
		t.Errorf("Full() = %q, want %q", got, want)
	}
}

func TestCompileExtensionTypesAndAggregates(t *testing.T) {
	schemaSrc := `
use extension 'postgis';
use extension 'vector';

scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32;
  longitude: float32;
};
scalar type Embedding extends sql "vector(1536)" as multi float32;
scalar type Citext extends sql "citext" as str;

type Place {
  required id: uuid;
  name: Citext;
  loc: Point;
  vec: Embedding;
  rating: float32;
}
`
	src, err := asl.Parse([]byte(schemaSrc))
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	ir, err := (&asl.Resolver{}).Resolve(src)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}

	// 1. Query with typed JSON dot-access on custom SQL Point
	stmt, err := aql.ParseString(`multi select Place { id, name, lat := .loc.latitude } filter .name = $name<Citext>;`)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	compiled, err := Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}
	if !strings.Contains(compiled.SQL, `(p.loc->>'latitude')::REAL`) {
		t.Errorf("expected dot access on Point scalar, got:\n%s", compiled.SQL)
	}

	// 2. Query with aggregate expression
	stmtAgg, err := aql.ParseString(`select Place { best := min(haversine(.loc.latitude, .loc.longitude, $target_lat<float32>, $target_lon<float32>)) };`)
	if err != nil {
		t.Fatalf("parse aggregate query: %v", err)
	}
	compiledAgg, err := Compile(stmtAgg, ir)
	if err != nil {
		t.Fatalf("compile aggregate query: %v", err)
	}
	if !strings.Contains(compiledAgg.SQL, `MIN(haversine(((p.loc->>'latitude')::REAL), ((p.loc->>'longitude')::REAL), $1::REAL, $2::REAL)) AS best`) {
		t.Errorf("expected aggregate expression compilation, got:\n%s", compiledAgg.SQL)
	}

	// 3. Query with subquery aggregate
	stmtSub, err := aql.ParseString(`select Place { id } filter (select count(Place filter .rating > 4.0)) > 0;`)
	if err != nil {
		t.Fatalf("parse subquery aggregate query: %v", err)
	}
	compiledSub, err := Compile(stmtSub, ir)
	if err != nil {
		t.Fatalf("compile subquery aggregate query: %v", err)
	}
	if !strings.Contains(compiledSub.SQL, `(SELECT COUNT(*) FROM (`) {
		t.Errorf("expected subquery count compilation, got:\n%s", compiledSub.SQL)
	}
}

