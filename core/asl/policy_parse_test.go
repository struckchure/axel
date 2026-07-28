package asl

import (
	"strings"
	"testing"
)

func resolveSrc(t *testing.T, src string) *SchemaIR {
	t.Helper()
	sf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return ir
}

func TestPolicyResolves(t *testing.T) {
	ir := resolveSrc(t, `
type KV {
  required key: str;
  value: json;
  expires_at: datetime;

  policy hide_expired for select using ( .expires_at is null or .expires_at >= now() );
  policy owner_all for all to app_user using ( .key = current_user ) with check ( .key = current_user );
}`)
	kv := ir.ObjectTypes["KV"]
	if kv == nil || len(kv.Policies) != 2 {
		t.Fatalf("want 2 policies, got %+v", kv)
	}

	p := kv.Policies[0]
	if p.Name != "hide_expired" || p.Command != "select" {
		t.Errorf("policy 0 = %+v", p)
	}
	// Predicates are now captured as raw native AQL; lowering to SQL is deferred to
	// the migration bridge.
	if p.UsingAQL != `.expires_at is null or .expires_at >= now()` {
		t.Errorf("usingAQL = %q", p.UsingAQL)
	}

	p2 := kv.Policies[1]
	if p2.Command != "all" || len(p2.Roles) != 1 || p2.Roles[0] != "app_user" {
		t.Errorf("policy 1 = %+v", p2)
	}
	if p2.CheckAQL != `.key = current_user` {
		t.Errorf("checkAQL = %q", p2.CheckAQL)
	}
}

func TestPolicyCapturesRawAQL(t *testing.T) {
	ir := resolveSrc(t, `
type Doc {
  required title: str;
  policy p for select using ( length(.title) > 0 and lower(.title) != 'x' );
}`)
	got := ir.ObjectTypes["Doc"].Policies[0].UsingAQL
	want := `length(.title) > 0 and lower(.title) != 'x'`
	if got != want {
		t.Errorf("usingAQL = %q, want %q", got, want)
	}
}

func TestPolicyErrors(t *testing.T) {
	// Field-existence is validated at bridge/compile time, not here, so an unknown
	// field does NOT error at resolve time (see the bridge policy tests).
	cases := map[string]string{
		"bad command": `type T { required a: str; policy p for frobnicate using ( .a = 1 ); }`,
		"no clause":   `type T { required a: str; policy p for select; }`,
	}
	for name, src := range cases {
		sf, err := Parse([]byte(src))
		if err != nil {
			continue // parse-level rejection is also acceptable
		}
		if _, err := (&Resolver{}).Resolve(sf); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestForDirectiveRunOnce(t *testing.T) {
	ir := resolveSrc(t, `
@for KV
function kv_gc() -> int64 { return 1; };

type KV { required key: str; }`)
	fn := ir.Functions["kv_gc"]
	if fn == nil || fn.RunOnceFor != "KV" {
		t.Fatalf("kv_gc RunOnceFor = %+v", fn)
	}
}

func TestPolicyInheritedFromAbstract(t *testing.T) {
	ir := resolveSrc(t, `
abstract type Soft {
  deleted_at: datetime;
  policy not_deleted for select using ( .deleted_at is null );
}
type Note extending Soft {
  required body: str;
}`)
	note := ir.ObjectTypes["Note"]
	if len(note.Policies) != 1 || note.Policies[0].Name != "not_deleted" {
		t.Fatalf("inherited policy missing: %+v", note.Policies)
	}
	if !strings.Contains(note.Policies[0].UsingAQL, `.deleted_at is null`) {
		t.Errorf("inherited usingAQL = %q", note.Policies[0].UsingAQL)
	}
}
