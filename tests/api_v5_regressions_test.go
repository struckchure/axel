package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/typescript"
)

const v5Schema = `
enum UserType { Admin, Runner, Vendor }
enum CategoryType { Restaurant, Grocery }

scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32 := ST_Y(__self__::geometry);
  longitude: float32 := ST_X(__self__::geometry);
};

type User {
  required id: uuid { constraint pk; };
  required email: str { constraint exclusive; };
  name: str;
  notification_token: str;
  multi roles: UserType;
}

type Vendor {
  required id: uuid { constraint pk; };
  required name: str;
  address: Point;
  multi category_types: CategoryType;
  multi link category: Category;
}

type Category {
  required id: uuid { constraint pk; };
  required name: str;
  link added_by: User;
}

type Product {
  required id: uuid { constraint pk; };
  required link vendor: Vendor;
}

type Order {
  required id: uuid { constraint pk; };
  link runner: User;
}
`

// Inside ON CONFLICT ... DO UPDATE two rows are in scope — the existing row and
// EXCLUDED — so a bare column on the right-hand side is ambiguous and Postgres
// rejects the statement with 42702. Verified against Postgres 17.
func TestConflictUpdateQualifiesFieldReference(t *testing.T) {
	c := compileAQL(t, v5Schema,
		`insert User { email := $email } unless conflict on .email else (update User set { name := $name? ?? .name });`)

	if !strings.Contains(c.SQL, `COALESCE($2::TEXT, "user".name)`) {
		t.Errorf("conflict update must qualify the target row:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "COALESCE($2::TEXT, name)") {
		t.Errorf("bare column reference is ambiguous inside DO UPDATE:\n%s", c.SQL)
	}
}

// An optional param in a junction membership test used to skip the null guard
// every other optional position gets, so an omitted value matched *nothing*
// (EXISTS over `IN (NULL)` is never true) rather than everything.
func TestOptionalParamGuardsMultiLinkMembership(t *testing.T) {
	c := compileAQL(t, v5Schema, `multi select Vendor { id } filter $category_id<uuid>? in .category;`)

	if !strings.Contains(c.SQL, "$1::UUID IS NULL OR EXISTS") {
		t.Errorf("optional membership test lost its null guard:\n%s", c.SQL)
	}
}

// A required param is compared unconditionally — the guard is for optionals only.
func TestRequiredParamMultiLinkMembershipUnguarded(t *testing.T) {
	c := compileAQL(t, v5Schema, `multi select Vendor { id } filter $cid<uuid> in .category;`)

	if strings.Contains(c.SQL, "IS NULL OR") {
		t.Errorf("required param must not be guarded:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `EXISTS (SELECT 1 FROM "vendor_category"`) {
		t.Errorf("expected EXISTS over the junction table:\n%s", c.SQL)
	}
}

// Reached through a link the array lives in another row, so the path compiles to
// a correlated subquery. `ANY (<subquery>)` is the subquery form of ANY, which
// compares text to text[]; the cast selects the array form instead.
func TestMembershipAcrossLinkCastsSubquery(t *testing.T) {
	c := compileAQL(t, v5Schema,
		`multi select Product { id } filter $ct<CategoryType>? in .vendor.category_types;`)

	if !strings.Contains(c.SQL, "::TEXT[])") {
		t.Errorf("subquery operand must be cast to the array type:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "LIMIT 1)))") {
		t.Errorf("operand should not be double-parenthesised:\n%s", c.SQL)
	}
}

// COUNT is bigint; every driver hands that back as a string to preserve
// precision, so an uncast count made the generated number typing a lie.
func TestCountCastsToInteger(t *testing.T) {
	c := compileAQL(t, v5Schema, `select count(Category filter .name = $n);`)

	if !strings.Contains(c.SQL, "COUNT(*)::INTEGER") {
		t.Errorf("count must be cast so it arrives as a number:\n%s", c.SQL)
	}
}

// An aggregate sub-select carries its type in AggFunc, not TypeName. The shape
// position used to resolve TypeName regardless and failed with `unknown type ""`,
// even though the identical expression worked in `order by`.
func TestCorrelatedAggregateAsShapeField(t *testing.T) {
	c := compileAQL(t, v5Schema,
		`multi select User { id, total := (select count(Order filter .runner = User.id)) };`)

	if !strings.Contains(c.SQL, "COUNT(*)::INTEGER") {
		t.Errorf("correlated aggregate did not compile:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "AS total") {
		t.Errorf("aggregate shape field lost its alias:\n%s", c.SQL)
	}
}

// The same aggregate must still work in order by, which was never broken.
func TestCorrelatedAggregateInOrderByStillWorks(t *testing.T) {
	c := compileAQL(t, v5Schema,
		`multi select User { id } order by (select count(Order filter .runner = User.id)) desc;`)

	if !strings.Contains(c.SQL, "ORDER BY (SELECT COUNT(*)") {
		t.Errorf("order-by aggregate regressed:\n%s", c.SQL)
	}
}

// Casts are postfix. The parser reports the failure at the *next* token, several
// characters past the real mistake, so the raw message rarely points at it.
func TestPrefixCastGetsAHint(t *testing.T) {
	_, err := aql.ParseString(
		`multi select Category { id, v := (select Vendor filter .id = <uuid>Category.added_by) };`)
	if err == nil {
		t.Fatal("expected a parse error for a prefix cast")
	}
	if !strings.Contains(err.Error(), "casts in AQL are postfix") {
		t.Errorf("error should name the construct, got: %v", err)
	}
}

// A valid postfix cast must not trip the hint.
func TestPostfixCastUnaffected(t *testing.T) {
	c := compileAQL(t, v5Schema, `multi select User { id } filter .email = $e<str>;`)
	if !strings.Contains(c.SQL, "$1::TEXT") {
		t.Errorf("postfix cast regressed:\n%s", c.SQL)
	}
}

// tsGen generates the TypeScript client for the schema above plus one query.
func tsGen(t *testing.T, name, file, query string) (models, queryFile, runner string) {
	t.Helper()
	ir := parseSchema(t, v5Schema)
	q := buildQueryDesc(t, ir, name, file, query)
	dir := t.TempDir()
	if err := codegen.Walk(codegen.FromSchemaIR(ir), []codegen.QueryDescriptor{q},
		&typescript.TsGenerator{}, &codegen.Context{OutDir: dir}); err != nil {
		t.Fatalf("ts walk: %v", err)
	}
	return readFile(t, filepath.Join(dir, "models.ts")),
		readFile(t, filepath.Join(dir, file[:len(file)-4]+".ts")),
		readFile(t, filepath.Join(dir, "runner.ts"))
}

// Row keys come from Postgres and the compiler aliases every column with its raw
// snake_case name, so a camelCased interface described an object that does not
// exist: reading row.notificationToken type-checked and was always undefined.
func TestRowInterfacesMatchSQLAliases(t *testing.T) {
	models, query, _ := tsGen(t, "getUser", "get_user.aql",
		`select User { name, notification_token } filter .email = $email;`)

	if !strings.Contains(query, "notification_token: string | null;") {
		t.Errorf("row interface must use the SQL alias:\n%s", query)
	}
	if strings.Contains(query, "notificationToken") {
		t.Errorf("row interface still camelCases a snake_case column:\n%s", query)
	}
	if !strings.Contains(models, "notification_token?: string | null;") {
		t.Errorf("model interface must use the column name:\n%s", models)
	}
	// Params are generator-owned on both sides, so they stay camelCase.
	if !strings.Contains(query, "email: string;") {
		t.Errorf("params interface changed unexpectedly:\n%s", query)
	}
}

// A single link's row key is its FK column, not "<name>Id" — that name matched
// no column, so the builder dropped every FK read and write silently.
func TestSingleLinkUsesJoinColumnName(t *testing.T) {
	models, _, _ := tsGen(t, "getCat", "get_cat.aql", `select Category { id } filter .id = $id;`)

	if !strings.Contains(models, "added_by?: string | null;") {
		t.Errorf("link field must be named after its FK column:\n%s", models)
	}
	if strings.Contains(models, "addedById") {
		t.Errorf("link field still uses the <name>Id form:\n%s", models)
	}
}

// A column declared with a custom scalar must resolve to that scalar's generated
// interface; the raw SQL type is not valid TypeScript.
func TestCustomScalarInQueryRowType(t *testing.T) {
	_, query, _ := tsGen(t, "updVendor", "upd_vendor.aql",
		`update Vendor filter .id = $id set { name := $name };`)

	if !strings.Contains(query, "address: Point | null;") {
		t.Errorf("custom scalar must type as its interface:\n%s", query)
	}
	if strings.Contains(query, "geography(") {
		t.Errorf("raw SQL type leaked into the row interface:\n%s", query)
	}
	// The scalar is imported alongside whatever else the row needs.
	importLine := ""
	for _, ln := range strings.Split(query, "\n") {
		if strings.HasPrefix(ln, "import type {") && strings.Contains(ln, `"./models"`) {
			importLine = ln
		}
	}
	if !strings.Contains(importLine, "Point") {
		t.Errorf("query file must import the scalar it references, got %q:\n%s", importLine, query)
	}
}

// runner.ts declares `export class Runner`, so a named import of a schema type
// called Runner made the file impossible to compile (TS2440/TS2395).
func TestRunnerNameCollisionAvoided(t *testing.T) {
	_, _, runner := tsGen(t, "getUser2", "get_user2.aql", `select User { id } filter .id = $id;`)

	if !strings.Contains(runner, `import type * as models from "./models";`) {
		t.Errorf("runner must import models under a namespace:\n%s", runner)
	}
	if strings.Contains(runner, `import type { RelationKeys`) {
		t.Errorf("runner must not import model names directly:\n%s", runner)
	}
	if !strings.Contains(runner, "User: models.User;") {
		t.Errorf("AxelSchema must reach model types through the namespace:\n%s", runner)
	}
}

// The actual collision: a schema type sharing the client class's name. Before
// the namespace import this file could not be compiled at all.
func TestSchemaTypeNamedRunnerCompiles(t *testing.T) {
	const schema = `
type Runner { required id: uuid { constraint pk; }; required name: str; }
type Queries { required id: uuid { constraint pk; }; }`

	ir := parseSchema(t, schema)
	q := buildQueryDesc(t, ir, "getRunner", "get_runner.aql", `select Runner { id, name } filter .id = $id;`)
	dir := t.TempDir()
	if err := codegen.Walk(codegen.FromSchemaIR(ir), []codegen.QueryDescriptor{q},
		&typescript.TsGenerator{}, &codegen.Context{OutDir: dir}); err != nil {
		t.Fatalf("ts walk: %v", err)
	}
	runner := readFile(t, filepath.Join(dir, "runner.ts"))
	barrel := readFile(t, filepath.Join(dir, "index.ts"))

	if !strings.Contains(runner, "Runner: models.Runner;") {
		t.Errorf("a schema type named Runner must be reached through the namespace:\n%s", runner)
	}
	// The class and the model interface must not both arrive as bare names.
	if strings.Contains(runner, `import type { Runner`) {
		t.Errorf("runner.ts imports a name it also declares:\n%s", runner)
	}
	// The barrel resolves the same clash by exporting the runner's names
	// explicitly, which beats `export *`, and aliasing the model side.
	if !strings.Contains(barrel, `export { Runner, Queries, type DB, type Shape, type ShapeResult } from "./runner";`) {
		t.Errorf("barrel must export runner names explicitly:\n%s", barrel)
	}
	if !strings.Contains(barrel, `type Queries as QueriesModel`) ||
		!strings.Contains(barrel, `type Runner as RunnerModel`) {
		t.Errorf("colliding model types must stay reachable under an alias:\n%s", barrel)
	}
}

// tsc rejects a ".ts" specifier unless allowImportingTsExtensions is set, and
// that flag additionally requires noEmit — so an emitting project could not
// typecheck the generated client at all.
func TestImportSpecifiersAreExtensionless(t *testing.T) {
	_, query, runner := tsGen(t, "getUser3", "get_user3.aql", `select User { id } filter .id = $id;`)

	for _, src := range []string{query, runner} {
		if strings.Contains(src, `.ts";`) {
			t.Errorf("generated import keeps a .ts extension:\n%s", src)
		}
	}
}

// A multi param binds one array; Bun's sql.unsafe would otherwise join it with
// commas and Postgres would reject the malformed literal.
func TestMultiParamGoesThroughArrayEncoder(t *testing.T) {
	_, query, runner := tsGen(t, "byRoles", "by_roles.aql",
		"var ( multi $emails: str; )\nmulti select User { id } filter .email in $emails;")

	if !strings.Contains(query, "_pgArray(params.emails)") {
		t.Errorf("array param must be encoded before binding:\n%s", query)
	}
	if !strings.Contains(query, `import { _pgArray } from "./runner";`) {
		t.Errorf("query file must import the encoder:\n%s", query)
	}
	if !strings.Contains(runner, "export function _pgArray") {
		t.Errorf("runner must export the encoder:\n%s", runner)
	}
}

// A scalar param must not be wrapped — only `multi` ones are arrays.
func TestScalarParamNotWrapped(t *testing.T) {
	_, query, _ := tsGen(t, "byEmail", "by_email.aql", `select User { id } filter .email = $email;`)

	if strings.Contains(query, "_pgArray") {
		t.Errorf("scalar param must bind directly:\n%s", query)
	}
}

// The builder resolved keys against properties only, so a link key matched
// nothing and was dropped: the insert succeeded with the FK unset, and the
// select came back without the field. Unknown keys now throw.
func TestBuilderResolvesLinkColumns(t *testing.T) {
	_, _, runner := tsGen(t, "getUser4", "get_user4.aql", `select User { id } filter .id = $id;`)

	for _, want := range []string{
		"function _writableColumn(",
		"function _requireColumn(",
		`l.name + "_id" === col`,
		"cols.push(`\"${_requireColumn(type, key)}\"`);",
		"is not a field on",
	} {
		if !strings.Contains(runner, want) {
			t.Errorf("runner.ts missing %q", want)
		}
	}
}
