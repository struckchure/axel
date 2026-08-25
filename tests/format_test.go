package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

// ── ASL ──────────────────────────────────────────────────────────────────────

func TestASLFormatExampleIdempotent(t *testing.T) {
	src, err := os.ReadFile("../examples/basic/default.asl")
	if err != nil {
		t.Fatal(err)
	}
	out, err := asl.Format(src)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	// Formatted output must parse.
	if _, err := asl.Parse([]byte(out)); err != nil {
		t.Fatalf("formatted output does not parse: %v", err)
	}
	// Formatting is idempotent.
	out2, err := asl.Format([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if out != out2 {
		t.Errorf("ASL formatting not idempotent:\n--- first ---\n%s\n--- second ---\n%s", out, out2)
	}
}

func TestASLFormatNormalizesWhitespace(t *testing.T) {
	messy := "type    A{required   id:uuid;name:str;}"
	out, err := asl.Format([]byte(messy))
	if err != nil {
		t.Fatal(err)
	}
	want := "type A {\n  required id:  uuid;\n  name:         str;\n}\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestASLFormatPreservesComments(t *testing.T) {
	src := "# header\n" +
		"type User {\n" +
		"  required email: str;  # login\n" +
		"  # age note\n" +
		"  required age: int32;\n" +
		"}\n"
	out, err := asl.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# header", "# login", "# age note"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatted output dropped comment %q:\n%s", want, out)
		}
	}
	// The trailing comment stays on the email line; the note stays above age.
	if !strings.Contains(out, "required email:  str;  # login") {
		t.Errorf("trailing comment not attached to its line:\n%s", out)
	}
	if !strings.Contains(out, "# age note\n  required age:    int32;") {
		t.Errorf("leading comment not kept above its field:\n%s", out)
	}
}

// An unparseable schema is a hard error (nothing to format safely).
func TestASLFormatParseErrorReturnsError(t *testing.T) {
	if _, err := asl.Format([]byte("type {")); err == nil {
		t.Error("expected a parse error for malformed schema")
	}
}

// ── AQL ──────────────────────────────────────────────────────────────────────

func TestAQLFormatIdempotent(t *testing.T) {
	queries := []string{
		"select User { id, email } filter .active = true order by .created_at desc limit $n;",
		"multi select Post { id, title, author: { name } } filter .published = true;",
		"insert User { email := $email, name := $name } unless conflict on .email;",
		"update User filter .id = $id set { name := $name, active := true };",
		"delete Session filter .expires_at < $now;",
		"select count(User filter .active = true);",
		"with ( b := (select Business filter .id = $id) ) multi select User filter .business = b.id;",
		"multi select Transaction filter .sender_id = $a and .reciever_id = $b and .status = $c and .created_at >= $since;",
		"update User filter .organization = $org and .email = $email and .active = true and .role = $role set { name := $name };",
		"delete Session filter .expires_at < now() and .user = $user and .revoked = false and .kind = $kind;",
	}
	for _, q := range queries {
		out, err := aql.Format([]byte(q))
		if err != nil {
			t.Fatalf("format %q: %v", q, err)
		}
		if _, err := aql.Parse([]byte(out)); err != nil {
			t.Fatalf("formatted %q does not parse: %v\n%s", q, err, out)
		}
		out2, err := aql.Format([]byte(out))
		if err != nil {
			t.Fatal(err)
		}
		if out != out2 {
			t.Errorf("AQL formatting not idempotent for %q:\n%s\n---\n%s", q, out, out2)
		}
	}
}

// A qualified enum reference must survive formatting (regression: the printer was
// missing the QualifiedIdent case and would drop it).
func TestAQLFormatKeepsEnumMemberReference(t *testing.T) {
	q := "select Transaction { id } filter .entity = TransactionActorEntity.ApiKey;"
	out, err := aql.Format([]byte(q))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "TransactionActorEntity.ApiKey") {
		t.Errorf("enum member reference dropped by formatter:\n%s", out)
	}
}

func TestAQLFormatPreservesComments(t *testing.T) {
	src := "# list active posts\n" +
		"select Post {\n" +
		"  id,  # pk\n" +
		"  title\n" +
		"} filter .active = true;"
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# list active posts", "# pk"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatted query dropped comment %q:\n%s", want, out)
		}
	}
}

// A filter that fits the width budget stays on one line — the formatter breaks
// boolean chains for readability, not on principle.
func TestAQLFormatKeepsShortFilterInline(t *testing.T) {
	out, err := aql.Format([]byte("multi select User filter .active = true and .age >= $min_age;"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "filter .active = true and .age >= $min_age;") {
		t.Errorf("short filter should stay on one line:\n%s", out)
	}
}

// The motivating query, written on a single line, must format to the canonical
// layout — and formatting the result again must not move anything.
func TestAQLFormatBreaksLongFilter(t *testing.T) {
	src := "with ( business := (select Business filter .id = $business_id); " +
		"api_keys := (multi select ApiKey filter .business = $business_id); ) " +
		"multi select Transaction filter (business is not null and " +
		"((.sender_id = business.id) or (.sender_id in api_keys.id) or " +
		"(.reciever_id in api_keys.id))) order by .updated_at desc " +
		"limit $limit<int32>? offset $offset<int32>?;"

	want := `with (
  business := (select Business filter .id = $business_id);
  api_keys := (multi select ApiKey filter .business = $business_id);
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
offset $offset<int32>?;
`
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Errorf("formatted output differs:\ngot:\n%s\nwant:\n%s", out, want)
	}
	out2, err := aql.Format([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if out2 != out {
		t.Errorf("not idempotent:\n%s\n---\n%s", out, out2)
	}
}

// A chain broken without wrapping parens hangs its continuations one level in,
// so they read as part of the filter rather than as new clauses — and, on an
// update, stay visually distinct from the `set` block that follows.
func TestAQLFormatHangsUnparenthesizedChain(t *testing.T) {
	src := "update User filter .organization = $org and .email = $email and " +
		".active = true and .role = $role set { name := $name };"
	want := `update User
filter .organization = $org
  and .email = $email
  and .active = true
  and .role = $role
set {
  name := $name
};
`
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Errorf("formatted output differs:\ngot:\n%s\nwant:\n%s", out, want)
	}
}

// Comments must stay attached to their operand when a filter is broken apart.
func TestAQLFormatKeepsCommentsInBrokenFilter(t *testing.T) {
	src := "multi select Transaction\n" +
		"filter (\n" +
		"  # only real senders\n" +
		"  business is not null # bound above\n" +
		"  and (.sender_id = business.id or .sender_id in api_keys.id or .reciever_id in api_keys.id)\n" +
		");"
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "  # only real senders\n  business is not null") {
		t.Errorf("own-line comment lost its operand:\n%s", out)
	}
	if !strings.Contains(out, "business is not null  # bound above") {
		t.Errorf("trailing comment lost its line:\n%s", out)
	}
}

func TestAQLFormatParseErrorReturnsError(t *testing.T) {
	if _, err := aql.Format([]byte("select {")); err == nil {
		t.Error("expected a parse error for malformed query")
	}
}

func TestAQLFormatTransactionBalance(t *testing.T) {
	src := `with (
  api_key := (
    multi select ApiKey { id }
    filter .id = $api_key_id<uuid>?
    or (
      .business.id = $business_id<uuid>?
      and .environment = $api_environment<ApiKeyEnvironment>?
    )
  );
)
select Transaction {
  success_debit := sum(.amount)<int64> filter .type = TransactionType.Debit and .status = TransactionStatus.Successful,
  pending_debit := sum(.amount)<int64> filter .type = TransactionType.Debit and .status = TransactionStatus.Pending,
  success_credit := sum(.amount)<int64> filter .type = TransactionType.Credit and .status = TransactionStatus.Successful,
  pending_credit := sum(.amount)<int64> filter .type = TransactionType.Credit and .status = TransactionStatus.Pending
}
filter (
  .currency = $currency
  and (.sender_id in api_key.id<str> or .reciever_id in api_key.id<str>)
);
`
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out != src {
		t.Errorf("formatted output differs:\ngot:\n%s\nwant:\n%s", out, src)
	}
	out2, err := aql.Format([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if out2 != out {
		t.Errorf("not idempotent:\n%s\n---\n%s", out, out2)
	}
}

func TestAQLFormatGroupByAndHaving(t *testing.T) {
	src := `multi select Transaction {
  status,
  total_volume := sum(.amount)<int64>,
  successful_volume := sum(.amount)<int64> filter .status = TransactionStatus.Successful,
  order_count := count()
}
filter .created_at >= $since
group by .status
having count() >= $min_orders and sum(.amount) > $min_volume
order by total_volume desc
limit $limit;
`
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out != src {
		t.Errorf("formatted output differs:\ngot:\n%s\nwant:\n%s", out, src)
	}
	out2, err := aql.Format([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if out2 != out {
		t.Errorf("not idempotent:\n%s\n---\n%s", out, out2)
	}
}

func TestAQLFormatVarBlock(t *testing.T) {
	src := `var (
  $status<TransactionStatus>?;
  $limit<int32>?;
)
multi select Transaction { id }
filter .status = $status
limit $limit;
`
	want := `var (
  $status<TransactionStatus>?;
  $limit<int32>?;
)

multi select Transaction { id }
filter .status = $status
limit $limit;
`
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Errorf("formatted output differs:\ngot:\n%s\nwant:\n%s", out, want)
	}
	out2, err := aql.Format([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if out2 != out {
		t.Errorf("not idempotent:\n%s\n---\n%s", out, out2)
	}
}

func TestAQLFormatDirectivesBlankLine(t *testing.T) {
	src := `@rel_load_strategy join
@name ListUsers
@response UserView
multi select User { id, email };`

	want := `@rel_load_strategy join
@name ListUsers
@response UserView

multi select User { id, email };
`
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Errorf("formatted output differs:\ngot:\n%s\nwant:\n%s", out, want)
	}
	// Verify idempotency
	out2, err := aql.Format([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if out2 != out {
		t.Errorf("directive formatting not idempotent:\n%s\n---\n%s", out, out2)
	}
}




// ── ASL member grouping ──────────────────────────────────────────────────────

// Members are printed in blocks: properties and links (then computed fields),
// constraints, indexes, policies, triggers — one blank line between blocks and
// none inside one, whatever order they were written in.
func TestASLFormatGroupsMembers(t *testing.T) {
	src := `type Post extends Base {
  policy owner for all using ( .title != '' );
  required content: str;
  index on (.content);
  computed excerpt := .content;
  trigger touch before update execute fn();
  required link author: User;
  constraint exclusive on (.title);
  index on (.title);
  required title: str;
}`
	want := `type Post extends Base {
  required content:      str;
  required link author:  User;
  required title:        str;
  computed excerpt := .content;

  constraint exclusive on (.title);

  index on (.content);
  index on (.title);

  policy owner for all using ( .title != '' );

  trigger touch before update execute fn();
}
`
	out, err := asl.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

// Field order decides column order, so fields keep the order they were written
// in — only the blocks around them move.
func TestASLFormatKeepsFieldOrder(t *testing.T) {
	src := "type A {\n  required c: str;\n\n  required a: str;\n  required b: str;\n}"
	out, err := asl.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := "type A {\n  required c:  str;\n  required a:  str;\n  required b:  str;\n}\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

// Reordering must carry each member's comments with it, including ones written
// inside a field body.
func TestASLFormatMovesCommentsWithMembers(t *testing.T) {
	src := `type A {
  # about the index
  index on (.name); # trailing on index
  required name: str {
    # inside the body
    default := 'x'; # trailing inside the body
  };
  # dangling before the brace
}`
	out, err := asl.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"required name:  str {\n    # inside the body\n    default := 'x';  # trailing inside the body",
		"# about the index\n  index on (.name);  # trailing on index",
		"# dangling before the brace\n}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lost or misplaced a comment; want to find:\n%s\n\ngot:\n%s", want, out)
		}
	}
}

// Regression: blank lines used to be derived from source line numbers, compared
// against the *start* line of the previous member. A multi-line field therefore
// produced a spurious blank line, the reformatted text rendered differently from
// the original, and Format's safety net returned the file unchanged — so `fmt`
// silently did nothing to any type containing a field with a body.
func TestASLFormatReformatsTypeWithMultiLineField(t *testing.T) {
	src := "type A {\n      required id: uuid {\n        default := gen_uuid();\n      };\n      required name: str;\n}\n"
	out, err := asl.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out == src {
		t.Fatal("formatter returned the source unchanged; the safety net bailed")
	}
	want := "type A {\n  required id:    uuid  { default := gen_uuid(); };\n  required name:  str;\n}\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestAQLFormatForLoopBulkInsert(t *testing.T) {
	src := `var multi $conditions: str? := {'Hot', 'Cold', 'Fragile', 'Frozen'}

for $condition in $conditions {
  insert PackageCondition {
    name := $condition,
    added_by := (select User filter .email = 'alice@example.com')
  } unless conflict;
}`
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := "var multi $conditions: str? := {'Hot', 'Cold', 'Fragile', 'Frozen'}\n\nfor $condition in $conditions {\n  insert PackageCondition {\n    name := $condition,\n    added_by := (select User filter .email = 'alice@example.com')\n  } unless conflict;\n}\n"
	if out != want {
		t.Errorf("formatted output differs:\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestASLFormatTabularAlignment(t *testing.T) {
	src := `type User extends Base {
  required email: EmailStr { constraint exclusive; };
  name: Citext { default := 'n/a'; };
  location: Point;
  bio_vec: Embedding;
  required role: Role;
  active: bool { default := true; };
}`
	want := `type User extends Base {
  required email:  EmailStr   { constraint exclusive; };
  name:            Citext     { default := 'n/a'; };
  location:        Point;
  bio_vec:         Embedding;
  required role:   Role;
  active:          bool       { default := true; };
}
`
	out, err := asl.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Errorf("tabular alignment differs:\ngot:\n%s\nwant:\n%s", out, want)
	}
	// Verify idempotence
	out2, err := asl.Format([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if out != out2 {
		t.Errorf("tabular formatting not idempotent:\n%s\nvs\n%s", out, out2)
	}
}

func TestASLFormatScalarStructuredTabularAlignment(t *testing.T) {
	src := `scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32;
  longitude: float32;
  elevation: float32;
};`
	want := `scalar type Point extends sql "geography(Point, 4326)" as {
  latitude:   float32;
  longitude:  float32;
  elevation:  float32;
}
`
	out, err := asl.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Errorf("scalar tabular alignment differs:\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestASLFormatReceiverFunctions(t *testing.T) {
	src := `scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32;
  longitude: float32;
};

function (p Point) deserialize() Point {
  return Point{ latitude: ST_Y(p::geometry), longitude: ST_X(p::geometry) };
};

function (p Point) serialize() {
  return ST_SetSRID(ST_MakePoint(p.longitude, p.latitude), 4326);
};
`
	out, err := asl.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := asl.Parse([]byte(out)); err != nil {
		t.Fatalf("receiver function output does not parse: %v\n%s", err, out)
	}
	out2, err := asl.Format([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if out != out2 {
		t.Errorf("receiver function formatting not idempotent:\n%s\nvs\n%s", out, out2)
	}
}

func TestASLFormatStructReturnSpacing(t *testing.T) {
	messy := `scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32;
  longitude: float32;
};

function (p Point) deserialize() Point {
  return Point {latitude:ST_Y(p::geometry), longitude:ST_X(p::geometry) };
};
`
	out, err := asl.Format([]byte(messy))
	if err != nil {
		t.Fatal(err)
	}
	want := `scalar type Point extends sql "geography(Point, 4326)" as {
  latitude:   float32;
  longitude:  float32;
}

function (p Point) deserialize() -> Point {
  return Point{ latitude: ST_Y(p::geometry), longitude: ST_X(p::geometry) };
};
`
	if out != want {
		t.Errorf("struct return spacing differs:\ngot:\n%s\nwant:\n%s", out, want)
	}
}


