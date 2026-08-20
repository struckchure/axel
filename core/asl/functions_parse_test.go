package asl

import (
	"strings"
	"testing"
)

func TestParseGeneralFunction(t *testing.T) {
	src := `
use extension 'unaccent';

@language plpgsql
@immutable
@strict
@parallel safe
function slugify(value: text, sep: text) -> text {
  return regexp_replace(lower(public.unaccent(value)), '\s+', sep, 'g');
};
`
	sf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(ir.Extensions) != 1 || ir.Extensions[0] != "unaccent" {
		t.Fatalf("extensions = %v, want [unaccent]", ir.Extensions)
	}

	fn := ir.Functions["slugify"]
	if fn == nil {
		t.Fatal("slugify not resolved")
	}
	if fn.Returns != "text" {
		t.Errorf("Returns = %q, want text (raw passthrough)", fn.Returns)
	}
	if len(fn.Params) != 2 || fn.Params[0].SQLType != "text" || fn.Params[1].SQLType != "text" {
		t.Errorf("Params = %+v, want two text params", fn.Params)
	}
	if fn.Language != "plpgsql" || fn.Volatility != "immutable" || !fn.Strict || fn.Parallel != "safe" {
		t.Errorf("attrs: lang=%q vol=%q strict=%v parallel=%q", fn.Language, fn.Volatility, fn.Strict, fn.Parallel)
	}
	if !strings.Contains(fn.ReturnSQL, "regexp_replace(lower(public.unaccent(value))") {
		t.Errorf("ReturnSQL not captured verbatim: %q", fn.ReturnSQL)
	}
}

// ASL scalar types still map through; raw Postgres types (and arrays) pass
// through untouched.
func TestFunctionTypeResolution(t *testing.T) {
	src := `
@language sql
function f(a: str, b: int32, c: jsonb, d: inet, e: text[]) -> bool {
  return a is not null;
};
`
	sf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	fn := ir.Functions["f"]
	want := []string{"TEXT", "INTEGER", "JSONB", "inet", "text[]"}
	for i, w := range want {
		if fn.Params[i].SQLType != w {
			t.Errorf("param %d SQLType = %q, want %q", i, fn.Params[i].SQLType, w)
		}
	}
	if fn.Returns != "BOOLEAN" {
		t.Errorf("Returns = %q, want BOOLEAN", fn.Returns)
	}
}

// A rewrite may call a declared function with a row-reference argument; it folds
// into the BEFORE trigger as slugify(NEW."title").
func TestRewriteFunctionCall(t *testing.T) {
	src := `
@language sql
function slugify(value: text) -> text { return lower(value); };

type Post {
  id: uuid;
  title: str;
  slug: str { rewrite create, update := slugify(__new__.title); };
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
	rw := ir.ObjectTypes["Post"].Properties["slug"].Rewrites
	if len(rw) != 1 {
		t.Fatalf("rewrites = %+v", rw)
	}
	if rw[0].ValueSQL != `slugify(NEW."title")` {
		t.Errorf("rewrite value = %q, want slugify(NEW.\"title\")", rw[0].ValueSQL)
	}
}

func TestFunctionDirectiveErrors(t *testing.T) {
	cases := map[string]string{
		"unknown directive":      "@bogus\nfunction f() -> text { return '1'; };",
		"bad parallel":           "@parallel sideways\nfunction f() -> text { return '1'; };",
		"conflicting volatility": "@immutable\n@volatile\nfunction f() -> text { return '1'; };",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			sf, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if _, err := (&Resolver{}).Resolve(sf); err == nil {
				t.Errorf("expected resolve error for %s", name)
			}
		})
	}
}

func TestFunctionFormatting(t *testing.T) {
	src := `@language sql
@volatile
@strict
function random_hex(len: int32) -> str { return substr(encode(gen_random_bytes(ceil(len / 2.0)::integer), 'hex'), 1, len); };
`
	formatted, err := Format([]byte(src))
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}
	want := `@language sql
@volatile
@strict
function random_hex(len: int32) -> str {
  return substr(encode(gen_random_bytes(ceil(len / 2.0)::integer), 'hex'), 1, len);
};
`
	if formatted != want {
		t.Errorf("formatted:\n%q\nwant:\n%q", formatted, want)
	}
}

