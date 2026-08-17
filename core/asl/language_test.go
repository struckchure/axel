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
