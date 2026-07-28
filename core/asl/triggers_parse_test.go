package asl

import (
	"strings"
	"testing"
)

func TestParseTriggerConstructs(t *testing.T) {
	src := `
abstract type Base {
  id: uuid { default := gen_uuid(); };
  updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();
  };
}

function log_changes() -> trigger {
  return NEW;
};

@language plpgsql
function raw_fn() -> trigger {
  return NEW;
};

type Application extends Base {
  name: str;
  trigger audit_changes after insert, update, delete do (
    insert AuditLog { actor := __new__.updated_by }
  );
  trigger touch before update execute raw_fn();
}
`
	sf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var fns, types int
	for _, d := range sf.Definitions {
		if d.Function != nil {
			fns++
		}
		if d.TypeDef != nil {
			types++
		}
	}
	if fns != 2 {
		t.Errorf("expected 2 functions, got %d", fns)
	}
	if types != 2 {
		t.Errorf("expected 2 types, got %d", types)
	}

	// Rewrite on Base.updated_at
	base := findType(sf, "Base")
	if base == nil {
		t.Fatal("Base not found")
	}
	var foundRewrite bool
	for _, m := range base.Members {
		if m.Field != nil && m.Field.Name == "updated_at" && m.Field.Body != nil {
			for _, it := range m.Field.Body.Items {
				if it.Rewrite != nil {
					foundRewrite = true
					if len(it.Rewrite.Events) != 1 || it.Rewrite.Events[0] != "update" {
						t.Errorf("rewrite events = %v", it.Rewrite.Events)
					}
					if it.Rewrite.Call == nil || it.Rewrite.Call.Func != "datetime_current" {
						t.Errorf("rewrite call = %v", it.Rewrite.Call)
					}
				}
			}
		}
	}
	if !foundRewrite {
		t.Error("rewrite not parsed on updated_at")
	}

	// Trigger with do-body on Application
	app := findType(sf, "Application")
	var doTrig, execTrig *TriggerDecl
	for _, m := range app.Members {
		if m.Trigger != nil {
			if m.Trigger.Do != nil {
				doTrig = m.Trigger
			}
			if m.Trigger.ExecFn != nil {
				execTrig = m.Trigger
			}
		}
	}
	if doTrig == nil {
		t.Fatal("do-body trigger not parsed")
	}
	if doTrig.Timing != "after" || len(doTrig.Events) != 3 {
		t.Errorf("do trigger timing/events = %q %v", doTrig.Timing, doTrig.Events)
	}
	if !strings.Contains(doTrig.Do.Raw, "insert AuditLog") || !strings.Contains(doTrig.Do.Raw, "__new__.updated_by") {
		t.Errorf("do-body raw did not reconstruct:\n%q", doTrig.Do.Raw)
	}
	if execTrig == nil || *execTrig.ExecFn != "raw_fn" {
		t.Errorf("execute trigger not parsed: %+v", execTrig)
	}

	// Function bodies: both use `return <expr>;`.
	var logFn, rawFn *FunctionDecl
	for _, d := range sf.Definitions {
		if d.Function == nil {
			continue
		}
		switch d.Function.Name {
		case "log_changes":
			logFn = d.Function
		case "raw_fn":
			rawFn = d.Function
		}
	}
	if logFn == nil || logFn.Return == nil || !strings.Contains(logFn.Return.Raw, "NEW") {
		t.Fatalf("log_changes return body not parsed: %+v", logFn)
	}
	if rawFn == nil || rawFn.Return == nil || !strings.Contains(rawFn.Return.Raw, "NEW") {
		t.Fatalf("raw_fn return body not parsed: %+v", rawFn)
	}
}

func TestResolveTriggerConstructs(t *testing.T) {
	src := `
abstract type Base {
  id: uuid { default := gen_uuid(); };
  updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();
  };
}
function log_changes() -> trigger {
  return NEW;
};
type Application extends Base {
  name: str { rewrite insert, update := __subject__.name; };
  trigger audit after insert, update, delete execute log_changes();
}
`
	sf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Function registered, returns trigger, AQL body captured.
	fn := ir.Functions["log_changes"]
	if fn == nil || fn.Returns != "trigger" || fn.Language != "plpgsql" {
		t.Fatalf("log_changes fn = %+v", fn)
	}

	app := ir.ObjectTypes["Application"]
	// Inherited updated_at rewrite → now()
	up := app.Properties["updated_at"]
	if up == nil || len(up.Rewrites) != 1 || up.Rewrites[0].ValueSQL != "now()" {
		t.Errorf("inherited updated_at rewrite = %+v", up)
	}
	// Own name rewrite → NEW."name"
	nm := app.Properties["name"]
	if nm == nil || len(nm.Rewrites) != 1 || nm.Rewrites[0].ValueSQL != `NEW."name"` {
		t.Errorf("name rewrite = %+v", nm)
	}
	// Trigger references the function
	if len(app.Triggers) != 1 || app.Triggers[0].Function != "log_changes" {
		t.Errorf("triggers = %+v", app.Triggers)
	}
}

// Rewrite values accept __new__/__old__/__subject__ row references and the
// `create` event alias for insert.
func TestResolveRewriteRowRefs(t *testing.T) {
	src := `
type Post {
  id: uuid;
  title: str;
  slug: str { rewrite create, update := __new__.title; };
  prev_status: str { rewrite update := __old__.status; };
  status: str;
}
`
	sf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	post := ir.ObjectTypes["Post"]

	slug := post.Properties["slug"]
	if len(slug.Rewrites) != 1 {
		t.Fatalf("slug rewrites: %+v", slug.Rewrites)
	}
	// `create` normalized to insert.
	if got := slug.Rewrites[0].Events; len(got) != 2 || got[0] != "insert" || got[1] != "update" {
		t.Errorf("create should normalize to insert, got events %v", got)
	}
	if slug.Rewrites[0].ValueSQL != `NEW."title"` {
		t.Errorf("__new__.title = %q", slug.Rewrites[0].ValueSQL)
	}

	prev := post.Properties["prev_status"]
	if len(prev.Rewrites) != 1 || prev.Rewrites[0].ValueSQL != `OLD."status"` {
		t.Errorf("__old__.status rewrite = %+v", prev.Rewrites)
	}
}

func TestResolveTriggerErrors(t *testing.T) {
	cases := map[string]string{
		"unknown function":  `type A { id: uuid; trigger t after insert execute nope(); }`,
		"bad rewrite event": `type A { id: uuid; x: str { rewrite delete := 'y'; }; }`,
		"not-trigger fn":    `function f() -> str { return '1'; }; type A { id: uuid; trigger t after insert execute f(); }`,
	}
	for name, src := range cases {
		sf, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if _, err := (&Resolver{}).Resolve(sf); err == nil {
			t.Errorf("%s: expected resolve error", name)
		}
	}
}

func findType(sf *SourceFile, name string) *TypeDef {
	for _, d := range sf.Definitions {
		if d.TypeDef != nil && d.TypeDef.Name == name {
			return d.TypeDef
		}
	}
	return nil
}
