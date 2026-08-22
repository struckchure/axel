package asl

import (
	"strings"
	"testing"
)

// TestResolveLanguageDeclarations exercises the declaration forms that make up
// a normal schema together.  It deliberately asserts the resolved IR rather
// than parser implementation details, so it protects the public ASL contract.
func TestResolveLanguageDeclarations(t *testing.T) {
	ir := resolveSrc(t, `
use extension 'uuid-ossp';
scalar type Email extending str;
enum Status { Draft, Published }
global required tenant_id: uuid;

abstract type Timestamped {
  created_at: datetime { default := datetime_current(); };
}

function audit_post() -> trigger { return NEW; };

type User extends Timestamped {
  required id: uuid { default := gen_uuid(); };
  required email: Email { constraint exclusive; };
  status: Status { default := Status.Draft; };
  computed label := .email ?? 'unknown';
  index on (.email, .created_at);
  constraint exclusive on (.email, .status) filter .status = Status.Draft;
}

type Post {
  required id: uuid;
  title: str { rewrite update := __new__.title; };
  required author: User;
  multi reviewers: User;
  trigger audit before insert execute audit_post();
}`)

	if got := ir.Extensions; len(got) != 1 || got[0] != "uuid-ossp" {
		t.Errorf("extensions = %v, want [uuid-ossp]", got)
	}
	if scalar := ir.ScalarTypes["Email"]; scalar == nil || scalar.Base != "str" || scalar.SQLType != "TEXT" {
		t.Errorf("Email scalar = %+v", scalar)
	}
	if enum := ir.EnumTypes["Status"]; enum == nil || strings.Join(enum.Values, ",") != "Draft,Published" {
		t.Errorf("Status enum = %+v", enum)
	}
	if len(ir.Globals) != 1 || ir.Globals[0].Name != "tenant_id" || !ir.Globals[0].Required || ir.Globals[0].SQLType != "UUID" {
		t.Errorf("globals = %+v", ir.Globals)
	}

	user := ir.ObjectTypes["User"]
	if user == nil || user.IsAbstract || user.Table != "user" {
		t.Fatalf("User = %+v", user)
	}
	if p := user.Properties["email"]; p == nil || p.SQLType != "TEXT" || !p.IsRequired || len(p.Constraints) != 1 {
		t.Errorf("email property = %+v", p)
	}
	if p := user.Properties["status"]; p == nil || p.EnumType != "Status" || p.Default != "'Draft'" {
		t.Errorf("status property = %+v", p)
	}
	if computed := user.Computed["label"]; computed == nil || computed.Expr != ".email??'unknown'" {
		t.Errorf("computed label = %+v", computed)
	}
	if len(user.Indexes) != 1 || strings.Join(user.Indexes[0].Columns, ",") != "email,created_at" {
		t.Errorf("indexes = %+v", user.Indexes)
	}
	if len(user.Constraints) != 1 || user.Constraints[0].FilterAQL != ".status = Status.Draft" {
		t.Errorf("constraints = %+v", user.Constraints)
	}

	post := ir.ObjectTypes["Post"]
	if single := post.Links["author"]; single == nil || single.IsMulti || single.TargetType != "User" || single.JoinColumn != "author" {
		t.Errorf("author link = %+v", single)
	}
	if multi := post.Links["reviewers"]; multi == nil || !multi.IsMulti || multi.JunctionTable != "post_reviewers" {
		t.Errorf("reviewers link = %+v", multi)
	}
	if fn := ir.Functions["audit_post"]; fn == nil || fn.Returns != "trigger" || fn.ReturnSQL != "NEW" {
		t.Errorf("audit_post function = %+v", fn)
	}
	if p := post.Properties["title"]; p == nil || len(p.Rewrites) != 1 || p.Rewrites[0].ValueSQL != `NEW."title"` {
		t.Errorf("title rewrites = %+v", p)
	}
	if len(post.Triggers) != 1 || post.Triggers[0].Function != "audit_post" || post.Triggers[0].Timing != "before" {
		t.Errorf("triggers = %+v", post.Triggers)
	}
}

func TestResolveLanguageValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unknown inherited type", `type Child extends Missing { id: uuid; }`, "extends unknown type"},
		{"inheritance cycle", `type A extends B { id: uuid; } type B extends A { id: uuid; }`, "inheritance cycle"},
		{"unknown property type", `type T { value: Missing; }`, "unknown type"},
		{"unknown link target", `type T { link owner: Missing; }`, "references unknown type"},
		{"invalid rewrite event", `type T { value: str { rewrite delete := 'x'; }; }`, "rewrite event"},
		{"invalid trigger function", `type T { id: uuid; trigger t before insert execute missing(); }`, "executes unknown function"},
		{"scalar fields on non-json", `scalar type S extends str { x: str; }`, "cannot define fields"},
		{"disallowed bool in json scalar", `scalar type S extends json { active: bool; }`, "type \"bool\" is not allowed"},
		{"disallowed uuid in json scalar", `scalar type S extends json { id: uuid; }`, "type \"uuid\" is not allowed"},
		{"disallowed nested json in json scalar", `scalar type S extends json { nested: json; }`, "type \"json\" is not allowed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, err = (&Resolver{}).Resolve(src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("resolve error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestTypedJsonScalars(t *testing.T) {
	src := `
scalar type Coordinate extends json {
  lat: str;
  lng: str;
}

scalar type ItemStats extends jsonb {
  score: float64;
  views: int32;
  multi tags: str;
}

type Location {
  id: uuid;
  name: str;
  coord: Coordinate;
  stats: ItemStats;
}
`
	ir := resolveSrc(t, src)
	coord := ir.ScalarTypes["Coordinate"]
	if coord == nil || coord.Base != "json" || coord.SQLType != "JSON" {
		t.Fatalf("Coordinate scalar = %+v", coord)
	}
	if len(coord.Fields) != 2 {
		t.Fatalf("Coordinate fields len = %d, want 2", len(coord.Fields))
	}
	if f := coord.Fields["lat"]; f == nil || f.AQLType != "str" || f.SQLType != "TEXT" || f.IsMulti {
		t.Errorf("lat field = %+v", f)
	}

	stats := ir.ScalarTypes["ItemStats"]
	if stats == nil || stats.Base != "jsonb" || stats.SQLType != "JSONB" {
		t.Fatalf("ItemStats scalar = %+v", stats)
	}
	if len(stats.Fields) != 3 {
		t.Fatalf("ItemStats fields len = %d, want 3", len(stats.Fields))
	}
	if f := stats.Fields["score"]; f == nil || f.AQLType != "float64" || f.SQLType != "DOUBLE PRECISION" {
		t.Errorf("score field = %+v", f)
	}
	if f := stats.Fields["tags"]; f == nil || f.AQLType != "str" || !f.IsMulti {
		t.Errorf("tags field = %+v", f)
	}

	loc := ir.ObjectTypes["Location"]
	if loc == nil {
		t.Fatalf("Location type missing")
	}
	if p := loc.Properties["coord"]; p == nil || p.AQLType != "Coordinate" || p.SQLType != "JSON" {
		t.Errorf("coord prop = %+v", p)
	}
	if p := loc.Properties["stats"]; p == nil || p.AQLType != "ItemStats" || p.SQLType != "JSONB" {
		t.Errorf("stats prop = %+v", p)
	}
}

func TestExtendingDeprecationAndFormat(t *testing.T) {
	src := `scalar type Coordinate extending json {
  lat: str;
  lng: str;
}

type Base {
  id: uuid;
}

type Place extending Base {
  coord: Coordinate;
}
`
	ir := resolveSrc(t, src)
	warnings := ValidateWarnings(ir)
	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want 2: %v", len(warnings), warnings)
	}

	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}
	if strings.Contains(formatted, "extending") {
		t.Errorf("formatted still contains 'extending':\n%s", formatted)
	}
	if !strings.Contains(formatted, "scalar type Coordinate extends json {") {
		t.Errorf("formatted missing Coordinate body:\n%s", formatted)
	}
	if !strings.Contains(formatted, "type Place extends Base {") {
		t.Errorf("formatted missing Place extends Base:\n%s", formatted)
	}
}

func TestExtendedScalarTypes(t *testing.T) {
	src := `
scalar type BaseCode extends str {
  constraint min_length(6);
  default := '000000';
}

scalar type Code extends BaseCode {
  constraint max_length(6);
}

scalar type AutoDate extends datetime {
  rewrite update := datetime_current();
}

type Product {
  required id: uuid { constraint pk; };
  code: Code;
  custom_code: Code {
    default := '999999';
    constraint exclusive;
  };
  updated_at: AutoDate;
}
`
	ir := resolveSrc(t, src)

	// Check BaseCode scalar
	baseCode := ir.ScalarTypes["BaseCode"]
	if baseCode == nil || baseCode.Base != "str" || baseCode.SQLType != "TEXT" {
		t.Fatalf("BaseCode scalar = %+v", baseCode)
	}
	if len(baseCode.Constraints) != 1 || baseCode.Constraints[0].Name != "min_length" || baseCode.Constraints[0].Args[0] != "6" {
		t.Errorf("BaseCode constraints = %+v", baseCode.Constraints)
	}
	if baseCode.Default != "'000000'" {
		t.Errorf("BaseCode default = %q, want '000000'", baseCode.Default)
	}

	// Check Code scalar (inherited min_length + added max_length + inherited default)
	code := ir.ScalarTypes["Code"]
	if code == nil || code.Base != "BaseCode" || code.SQLType != "TEXT" {
		t.Fatalf("Code scalar = %+v", code)
	}
	if len(code.Constraints) != 2 {
		t.Fatalf("Code constraints len = %d, want 2: %+v", len(code.Constraints), code.Constraints)
	}
	if code.Constraints[0].Name != "min_length" || code.Constraints[1].Name != "max_length" {
		t.Errorf("Code constraints = %+v", code.Constraints)
	}
	if code.Default != "'000000'" {
		t.Errorf("Code default = %q, want '000000'", code.Default)
	}

	// Check Product properties
	product := ir.ObjectTypes["Product"]
	if product == nil {
		t.Fatalf("Product type missing")
	}

	// product.code inherits min_length, max_length, default
	pCode := product.Properties["code"]
	if pCode == nil {
		t.Fatalf("Product.code missing")
	}
	if pCode.AQLType != "Code" || pCode.SQLType != "TEXT" {
		t.Errorf("Product.code types = %s / %s", pCode.AQLType, pCode.SQLType)
	}
	if pCode.Default != "'000000'" {
		t.Errorf("Product.code default = %q, want '000000'", pCode.Default)
	}
	if len(pCode.Constraints) != 2 {
		t.Fatalf("Product.code constraints len = %d, want 2: %+v", len(pCode.Constraints), pCode.Constraints)
	}

	// product.custom_code overrides default, adds exclusive constraint
	pCustom := product.Properties["custom_code"]
	if pCustom == nil {
		t.Fatalf("Product.custom_code missing")
	}
	if pCustom.Default != "'999999'" {
		t.Errorf("Product.custom_code default = %q, want '999999'", pCustom.Default)
	}
	if len(pCustom.Constraints) != 3 {
		t.Fatalf("Product.custom_code constraints len = %d, want 3: %+v", len(pCustom.Constraints), pCustom.Constraints)
	}
	if pCustom.Constraints[2].Name != "exclusive" {
		t.Errorf("Product.custom_code 3rd constraint = %+v, want exclusive", pCustom.Constraints[2])
	}

	// product.updated_at inherits rewrite with Origin = Product
	pUpdated := product.Properties["updated_at"]
	if pUpdated == nil {
		t.Fatalf("Product.updated_at missing")
	}
	if len(pUpdated.Rewrites) != 1 {
		t.Fatalf("Product.updated_at rewrites len = %d, want 1", len(pUpdated.Rewrites))
	}
	if pUpdated.Rewrites[0].Origin != "Product" || pUpdated.Rewrites[0].ValueSQL != "now()" {
		t.Errorf("Product.updated_at rewrite = %+v", pUpdated.Rewrites[0])
	}

	// Check Format
	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}
	if !strings.Contains(formatted, "scalar type Code extends BaseCode {") ||
		!strings.Contains(formatted, "constraint max_length(6);") {
		t.Errorf("formatted missing Code body:\n%s", formatted)
	}
}

func TestDefaultFunctionWithArgs(t *testing.T) {
	src := `
function random_hex(len: int32) -> str { return substr(encode(gen_random_bytes(ceil(len / 2.0)::integer), 'hex'), 1, len); };

scalar type Code extends str {
  constraint min_length(6);
  constraint max_length(6);
  default := random_hex(6);
}

type User {
  required id: uuid { constraint pk; };
  code: Code;
}
`
	ir := resolveSrc(t, src)
	code := ir.ScalarTypes["Code"]
	if code == nil {
		t.Fatal("Code scalar not resolved")
	}
	if code.Default != "random_hex(6)" {
		t.Errorf("Code default = %q, want %q", code.Default, "random_hex(6)")
	}

	userCode := ir.ObjectTypes["User"].Properties["code"]
	if userCode == nil || userCode.Default != "random_hex(6)" {
		t.Errorf("User.code default = %+v, want random_hex(6)", userCode)
	}

	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}
	if !strings.Contains(formatted, "default := random_hex(6);") {
		t.Errorf("formatted output missing function default:\n%s", formatted)
	}
}

func TestDefaultFunctionArgumentTypeMismatch(t *testing.T) {
	src := `
function random_hex(len: int32) -> str { return 'hex'; };

scalar type Code extends str {
  default := random_hex('6');
}
`
	sf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	_, err = (&Resolver{}).Resolve(sf)
	if err == nil {
		t.Fatal("expected resolve error for passing string '6' to int32 parameter, got nil")
	}
	if !strings.Contains(err.Error(), `function "random_hex" argument 1 expects int32, got str ('6')`) {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestFormatGrouping(t *testing.T) {
	src := `use extension 'uuid-ossp';
use extension 'pgcrypto';

global current_user: uuid;
global required tenant_id: str;

scalar type Email extends str;
scalar type Slug extends str;

enum Status { Draft, Published, Archived }
enum Role { Admin, Member, Guest }

scalar type Code extends str {
  constraint min_length(6);
  constraint max_length(6);
}

type User {
  required id: uuid {
    constraint pk;
  };
  email: Email;
  status: Status;
}

type Organization {
  required id: uuid {
    constraint pk;
  };
  slug: Slug;
}
`
	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	if formatted != src {
		t.Errorf("Format output mismatch.\n--- GOT ---\n%s\n--- WANT ---\n%s", formatted, src)
	}
}

func TestFormatGroupingWithSpacingInput(t *testing.T) {
	// Input with excessive blank lines between enums and scalars
	src := `
enum Status { Draft, Published }

enum Role { Admin, Member }

scalar type Email extends str;

scalar type Slug extends str;
`
	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	want := `enum Status { Draft, Published }
enum Role { Admin, Member }

scalar type Email extends str;
scalar type Slug extends str;
`
	if formatted != want {
		t.Errorf("Format output mismatch.\n--- GOT ---\n%s\n--- WANT ---\n%s", formatted, want)
	}
}

func TestFormatGroupingWithComments(t *testing.T) {
	src := `# Auth roles
enum Role { Admin, Member }
enum Status { Active, Suspended }  # Account state

# Next section
enum Priority { Low, High }
`
	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	want := `# Auth roles
enum Role { Admin, Member }
enum Status { Active, Suspended }  # Account state

# Next section
enum Priority { Low, High }
`
	if formatted != want {
		t.Errorf("Format output mismatch.\n--- GOT ---\n%s\n--- WANT ---\n%s", formatted, want)
	}
}

func TestFormatEnumWrapping(t *testing.T) {
	// A long enum that exceeds 80 characters
	src := `enum OrderStatus { Pending, Processing, PaymentConfirmed, Packaging, Shipped, OutForDelivery, Delivered, Cancelled, Refunded }`
	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	want := `enum OrderStatus {
  Pending,
  Processing,
  PaymentConfirmed,
  Packaging,
  Shipped,
  OutForDelivery,
  Delivered,
  Cancelled,
  Refunded,
}
`
	if formatted != want {
		t.Errorf("Format output mismatch.\n--- GOT ---\n%s\n--- WANT ---\n%s", formatted, want)
	}
}

func TestFormatFunctionWrapping(t *testing.T) {
	// A function with a long parameter signature that exceeds 80 characters
	src := `@language sql
function calculate_complex_order_discount(customer_id: uuid, order_total: decimal, loyalty_tier: str, discount_rate: float64) -> decimal {
  return order_total * (1.0 - discount_rate);
};
`
	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	want := `@language sql
function calculate_complex_order_discount(
  customer_id: uuid,
  order_total: decimal,
  loyalty_tier: str,
  discount_rate: float64
) -> decimal {
  return order_total * (1.0 - discount_rate);
};
`
	if formatted != want {
		t.Errorf("Format output mismatch.\n--- GOT ---\n%s\n--- WANT ---\n%s", formatted, want)
	}
}

func TestFormatTriggerMultiLine(t *testing.T) {
	src := `type Application {
  required id: uuid {
    constraint pk;
  };
  required name: str;

  trigger audit after insert, update, delete do (
    insert AuditLog {
      table_name := 'application',
      action := event,
      new_data := to_jsonb(__new__)
    }
  );
  trigger touch before update execute slugify_name();
}
`
	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	if formatted != src {
		t.Errorf("Format output mismatch.\n--- GOT ---\n%s\n--- WANT ---\n%s", formatted, src)
	}
}

func TestFormatTriggerOrderAuditLog(t *testing.T) {
	src := `type Order {
  required user: User;

  trigger audit after insert, update do (
    insert AuditLog {
      entity_id := __new__.id,
      entity_type := __new__.id,
      actor := __new__.user.id,
      action := event,
      old := to_jsonb(__new__),
      new := to_jsonb(__new__)
    }
  );
}
`
	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	if formatted != src {
		t.Errorf("Format output mismatch.\n--- GOT ---\n%s\n--- WANT ---\n%s", formatted, src)
	}
}







