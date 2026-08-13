package tests

import (
	"strings"
	"testing"

	axel "github.com/struckchure/axel/core"
	"github.com/struckchure/axel/core/asl"
)

// lowerFunctions resolves a schema and lowers its functions, returning the
// lowering error (where inline AQL is compiled) rather than failing the test.
func lowerFunctions(t *testing.T, schema string) ([]axel.Function, error) {
	t.Helper()
	fns, _, err := axel.SchemaIRToFunctionsAndTriggers(parseSchema(t, schema))
	return fns, err
}

// A function body may embed an AQL query as an aql`…` literal. It compiles to
// SQL at migration-generation time and is inlined as a Postgres string literal —
// so the hand-written SQL string and the AQL form emit the same migration.
func TestInlineAQLInFunctionBody(t *testing.T) {
	schema := `
use extension 'pg_cron';

type KV {
  required key: str;
  expires_at: datetime;
}

@for KV
function kv_gc() -> int64 {
  return cron.schedule('kv-cron', '0 * * * *', aql` + "`delete KV filter .expires_at < now()`" + `);
};
`
	up := genUpFull(t, schema)

	want := `RETURN cron.schedule('kv-cron', '0 * * * *', 'DELETE FROM "kv" k WHERE k.expires_at < now();');`
	if !strings.Contains(up, want) {
		t.Errorf("inline aql not lowered to a SQL literal:\nwant %s\ngot:\n%s", want, up)
	}
	// @for still applies: the function is invoked once in the migration that adds it.
	if !strings.Contains(up, `SELECT "kv_gc"();`) {
		t.Errorf("@for run-once invocation missing:\n%s", up)
	}
}

// Single quotes in the compiled SQL are doubled so the literal stays well-formed.
func TestInlineAQLEscapesQuotes(t *testing.T) {
	schema := `
type KV {
  required key: str;
}

function kv_purge() -> int64 {
  return cron.schedule('kv', '0 * * * *', aql` + "`delete KV filter .key = 'temp'`" + `);
};
`
	up := genUpFull(t, schema)
	if !strings.Contains(up, `k.key = ''temp''`) {
		t.Errorf("quotes not doubled inside the literal:\n%s", up)
	}
}

// The rest of the return expression — including plain SQL string arguments — is
// untouched, and several inline queries in one body each lower independently.
func TestInlineAQLMultipleLiterals(t *testing.T) {
	schema := `
type KV {
  required key: str;
  expires_at: datetime;
}

function kv_two() -> int64 {
  return both(aql` + "`delete KV filter .expires_at < now()`" + `, 'literal', aql` + "`select KV { key }`" + `);
};
`
	up := genUpFull(t, schema)
	for _, want := range []string{
		`both('DELETE FROM "kv" k WHERE k.expires_at < now();'`,
		`, 'literal', 'SELECT k.key AS key FROM "kv" k LIMIT 1;')`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("missing %q:\n%s", want, up)
		}
	}
}

// A parameterized query has nothing to bind to once inlined, so it is rejected
// rather than emitting SQL with dangling $1 placeholders.
func TestInlineAQLRejectsParams(t *testing.T) {
	schema := `
type KV {
  required key: str;
}

function kv_gc() -> int64 {
  return run(aql` + "`delete KV filter .key = $key<str>`" + `);
};
`
	if _, err := lowerFunctions(t, schema); err == nil {
		t.Fatal("expected an error for a parameterized inline query")
	} else if !strings.Contains(err.Error(), "parameters") {
		t.Errorf("unexpected error: %v", err)
	}
}

// An unknown type inside an inline query fails at generation time, not at runtime.
func TestInlineAQLUnknownType(t *testing.T) {
	schema := `
type KV { required key: str; }

function kv_gc() -> int64 {
  return run(aql` + "`delete Nope filter .key = 'x'`" + `);
};
`
	if _, err := lowerFunctions(t, schema); err == nil {
		t.Fatal("expected an error for an unknown type in an inline query")
	}
}

// A backtick literal without the aql tag is a parse error — the prefix is what
// marks the span as a query.
func TestBacktickRequiresAQLPrefix(t *testing.T) {
	schema := `
function kv_gc() -> int64 {
  return run(` + "`delete KV`" + `);
};
`
	if _, err := asl.Parse([]byte(schema)); err == nil {
		t.Fatal("expected a parse error for an untagged backtick literal")
	}
}

// The formatter round-trips the literal verbatim rather than printing the
// lowered form.
func TestFormatPreservesInlineAQL(t *testing.T) {
	schema := "function kv_gc() -> int64 {\n  return run(aql`delete KV filter .expires_at < now()`);\n};\n"
	out, err := asl.Format([]byte(schema))
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if !strings.Contains(out, "aql`delete KV filter .expires_at < now()`") {
		t.Errorf("formatter did not preserve the inline query:\n%s", out)
	}
}
