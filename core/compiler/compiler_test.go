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
  latitude: float32 := ST_Y(__self__::geometry);
  longitude: float32 := ST_X(__self__::geometry);
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

	// 1. Query with custom SQL dot-access on Point
	stmt, err := aql.ParseString(`multi select Place { id, name, lat := .loc.latitude } filter .name = $name<Citext>;`)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	compiled, err := Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}
	if !strings.Contains(compiled.SQL, `(ST_Y(p.loc::geometry)) AS lat`) {
		t.Errorf("expected custom SQL dot access on Point scalar, got:\n%s", compiled.SQL)
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
	if !strings.Contains(compiledAgg.SQL, `MIN(haversine(ST_Y(p.loc::geometry), ST_X(p.loc::geometry), $1::REAL, $2::REAL)) AS best`) {
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
	// COUNT is bigint, which the drivers hand back as a string; the cast keeps the
	// generated Promise<number>/int64 typing honest.
	if !strings.Contains(compiledSub.SQL, `(SELECT COUNT(*)::INTEGER FROM (`) {
		t.Errorf("expected subquery count compilation, got:\n%s", compiledSub.SQL)
	}

	// 4. Query with explicit <geography> and <Point> casts
	stmtCast, err := aql.ParseString(`multi select Place { id } filter ST_DWithin(.loc, ST_MakePoint($lng<float32>, $lat<float32>)<geography>, 1000.0);`)
	if err != nil {
		t.Fatalf("parse cast query: %v", err)
	}
	compiledCast, err := Compile(stmtCast, ir)
	if err != nil {
		t.Fatalf("compile cast query: %v", err)
	}
	if !strings.Contains(compiledCast.SQL, `(ST_MakePoint($1::REAL, $2::REAL))::geography`) {
		t.Errorf("expected geography cast, got:\n%s", compiledCast.SQL)
	}

	// 5. Multi-link membership with multi-row subquery and with-binding
	schemaMultiLinkSrc := `
use extension 'postgis';
scalar type Point extends sql "geography(Point, 4326)";
type Address {
  required id: uuid { constraint pk; };
  point: Point;
}
type Coverage {
  required id: uuid { constraint pk; };
  multi addresses: Address;
}
`
	multiLinkIR, err := (&asl.Resolver{}).Resolve(aslMustParse(t, schemaMultiLinkSrc))
	if err != nil {
		t.Fatalf("resolve multi-link schema: %v", err)
	}

	// 5a. Subquery in multi-link
	stmtSubMem, err := aql.ParseString(`
multi select Coverage { id }
filter (
  multi select Address
  filter ST_DWithin(.point, ST_MakePoint($lng<float32>, $lat<float32>)<geography>, 5000.0)
) in .addresses;
`)
	if err != nil {
		t.Fatalf("parse subquery membership: %v", err)
	}
	compiledSubMem, err := Compile(stmtSubMem, multiLinkIR)
	if err != nil {
		t.Fatalf("compile subquery membership: %v", err)
	}
	if !strings.Contains(compiledSubMem.SQL, `EXISTS (SELECT 1 FROM "coverage_addresses" jt WHERE jt.coverage = c.id AND jt.address IN (SELECT a.id FROM "address" a WHERE ST_DWithin(a.point, (ST_MakePoint($1::REAL, $2::REAL))::geography, 5000.0)))`) {
		t.Errorf("expected correlated EXISTS with IN subquery, got:\n%s", compiledSubMem.SQL)
	}

	// 5b. With-binding in multi-link
	stmtWithMem, err := aql.ParseString(`
with (
  nearby := (
    multi select Address
    filter ST_DWithin(.point, ST_MakePoint($lng<float32>, $lat<float32>)<geography>, 5000.0)
  );
)
multi select Coverage { id }
filter nearby in .addresses;
`)
	if err != nil {
		t.Fatalf("parse with-binding membership: %v", err)
	}
	compiledWithMem, err := Compile(stmtWithMem, multiLinkIR)
	if err != nil {
		t.Fatalf("compile with-binding membership: %v", err)
	}
	if !strings.Contains(compiledWithMem.SQL, `EXISTS (SELECT 1 FROM "coverage_addresses" jt WHERE jt.coverage = c.id AND jt.address IN (SELECT _with_nearby.id FROM _with_nearby))`) {
		t.Errorf("expected correlated EXISTS with with-binding CTE, got:\n%s", compiledWithMem.SQL)
	}
}

func aslMustParse(t *testing.T, src string) *asl.SourceFile {
	sf, err := asl.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse asl: %v", err)
	}
	return sf
}

func TestCompileArithmeticExpressions(t *testing.T) {
	schemaSrc := `
type Account {
  required id: uuid { constraint pk; };
  balance: int64;
}
type Product {
  required id: uuid { constraint pk; };
  price: float64;
  quantity: int32;
  discount: float64;
  computed gross_total := .price * .quantity;
  computed net_total := (.price * .quantity) - .discount;
}
`
	ir, err := (&asl.Resolver{}).Resolve(aslMustParse(t, schemaSrc))
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}

	// 1. Arithmetic in computed shape fields
	stmt1, err := aql.ParseString(`
multi select Product {
  id,
  total := .price * .quantity + 10.5,
  negated := - .discount
};`)
	if err != nil {
		t.Fatalf("parse stmt1: %v", err)
	}
	compiled1, err := Compile(stmt1, ir)
	if err != nil {
		t.Fatalf("compile stmt1: %v", err)
	}
	if !strings.Contains(compiled1.SQL, `(p.price * p.quantity + 10.5) AS total`) {
		t.Errorf("expected price * quantity + 10.5 in SQL, got:\n%s", compiled1.SQL)
	}
	if !strings.Contains(compiled1.SQL, `(-p.discount) AS negated`) {
		t.Errorf("expected -p.discount in SQL, got:\n%s", compiled1.SQL)
	}

	// 2. Arithmetic in Filter / Order By
	stmt2, err := aql.ParseString(`
multi select Product { id }
filter .price * .quantity >= 100.0 - $rebate
order by .price * .quantity desc;`)
	if err != nil {
		t.Fatalf("parse stmt2: %v", err)
	}
	compiled2, err := Compile(stmt2, ir)
	if err != nil {
		t.Fatalf("compile stmt2: %v", err)
	}
	if !strings.Contains(compiled2.SQL, `WHERE p.price * p.quantity >= 100.0 - $1`) {
		t.Errorf("expected WHERE with arithmetic, got:\n%s", compiled2.SQL)
	}
	if !strings.Contains(compiled2.SQL, `ORDER BY p.price * p.quantity DESC`) {
		t.Errorf("expected ORDER BY with arithmetic, got:\n%s", compiled2.SQL)
	}

	// 3. Arithmetic in Update assignment
	stmt3, err := aql.ParseString(`
update Account filter .id = $id<uuid>
set { balance := .balance - $amount<int64> };`)
	if err != nil {
		t.Fatalf("parse stmt3: %v", err)
	}
	compiled3, err := Compile(stmt3, ir)
	if err != nil {
		t.Fatalf("compile stmt3: %v", err)
	}
	if !strings.Contains(compiled3.SQL, `balance = a.balance - $1::BIGINT`) {
		t.Errorf("expected UPDATE assignment with arithmetic, got:\n%s", compiled3.SQL)
	}

	// 4. ASL computed properties expanded in AQL shape
	stmt4, err := aql.ParseString(`
multi select Product { id, gross_total, net_total };`)
	if err != nil {
		t.Fatalf("parse stmt4: %v", err)
	}
	compiled4, err := Compile(stmt4, ir)
	if err != nil {
		t.Fatalf("compile stmt4: %v", err)
	}
	if !strings.Contains(compiled4.SQL, `(p.price * p.quantity) AS gross_total`) {
		t.Errorf("expected gross_total expansion, got:\n%s", compiled4.SQL)
	}
	if !strings.Contains(compiled4.SQL, `((p.price * p.quantity) - p.discount) AS net_total`) {
		t.Errorf("expected net_total expansion, got:\n%s", compiled4.SQL)
	}
}

