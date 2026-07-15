package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/lsp"
)

const lspSchema = `
type User { required id: uuid; required email: str; }
type Project {
  required id: uuid;
  required name: str;
  required link owner: User;
  multi link members: User;
}
`

func offsetOf(t *testing.T, text, sub string) int {
	t.Helper()
	i := strings.Index(text, sub)
	if i < 0 {
		t.Fatalf("substring %q not found in %q", sub, text)
	}
	return i + 1 // land inside the token
}

func TestSchemaDiagnostics(t *testing.T) {
	// Unknown scalar type on a property → resolve error, ranged at "Nope".
	diags := lsp.SchemaDiagnostics("type User {\n  required role: Nope;\n}\n")
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Message, "Nope") {
		t.Errorf("message %q should mention Nope", diags[0].Message)
	}
	if diags[0].Range.Start.Line != 1 {
		t.Errorf("diagnostic should be on line 1 (the role field), got %d", diags[0].Range.Start.Line)
	}
	// A valid schema yields no diagnostics.
	if d := lsp.SchemaDiagnostics(lspSchema); len(d) != 0 {
		t.Errorf("valid schema should have no diagnostics, got %+v", d)
	}
}

func TestQueryDiagnostics(t *testing.T) {
	schema := parseSchema(t, lspSchema)

	// Unknown field in a shape.
	diags := lsp.QueryDiagnostics("select Project { id, nope };", schema)
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "nope") {
		t.Fatalf("expected an unknown-field diagnostic, got %+v", diags)
	}
	// Valid query → none.
	if d := lsp.QueryDiagnostics("select Project { id, name };", schema); len(d) != 0 {
		t.Errorf("valid query should have no diagnostics, got %+v", d)
	}
	// No schema → parse-only (a well-formed query yields nothing).
	if d := lsp.QueryDiagnostics("select Project { id };", nil); len(d) != 0 {
		t.Errorf("schemaless valid query should have no diagnostics, got %+v", d)
	}
	// No schema → still reports parse errors.
	if d := lsp.QueryDiagnostics("select Project { id ", nil); len(d) != 1 {
		t.Errorf("expected a parse diagnostic, got %+v", d)
	}
}

func TestSymbols(t *testing.T) {
	syms := lsp.SchemaSymbols(lspSchema)
	names := map[string]int{}
	for _, s := range syms {
		names[s.Name] = len(s.Children)
	}
	if names["User"] != 2 {
		t.Errorf("User should have 2 field children, got %d", names["User"])
	}
	if _, ok := names["Project"]; !ok {
		t.Errorf("Project symbol missing: %+v", names)
	}

	q := "multi select Project { id } limit $limit offset $offset;"
	qsyms := lsp.QuerySymbols(q)
	if len(qsyms) != 1 {
		t.Fatalf("expected one query symbol, got %d", len(qsyms))
	}
	var params []string
	for _, c := range qsyms[0].Children {
		if strings.HasPrefix(c.Name, "$") {
			params = append(params, c.Name)
		}
	}
	if len(params) != 2 {
		t.Errorf("expected 2 param symbols, got %v", params)
	}
}

func TestHover(t *testing.T) {
	schema := parseSchema(t, lspSchema)
	q := "select Project { name, owner };"

	if h := lsp.QueryHover(q, offsetOf(t, q, "Project"), schema); h == nil || !strings.Contains(h.Contents, "type Project") {
		t.Errorf("hover on type should show the type; got %+v", h)
	}
	if h := lsp.QueryHover(q, offsetOf(t, q, "owner"), schema); h == nil || !strings.Contains(h.Contents, "owner: User") {
		t.Errorf("hover on link should show owner: User; got %+v", h)
	}
}

func TestDefinition(t *testing.T) {
	schema := parseSchema(t, lspSchema)
	q := "select Project { id };"
	loc := lsp.QueryDefinition(q, offsetOf(t, q, "Project"), schema, "file:///schema.asl", lspSchema)
	if loc == nil {
		t.Fatal("expected a definition location for Project")
	}
	if loc.URI != "file:///schema.asl" {
		t.Errorf("definition URI = %q", loc.URI)
	}
	// The schema declares Project on line 2 (0-based) in lspSchema.
	if loc.Range.Start.Line == 0 {
		t.Errorf("definition should point at the Project declaration line, got line 0")
	}
}

func TestCompletion(t *testing.T) {
	schema := parseSchema(t, lspSchema)

	// Inside a shape → the type's fields (and the * splat).
	inShape := "select Project { "
	labels := completionLabels(lsp.QueryCompletion(inShape, len(inShape), schema))
	for _, want := range []string{"*", "id", "name", "owner", "members"} {
		if !labels[want] {
			t.Errorf("shape completion missing %q; got %v", want, keys(labels))
		}
	}

	// After `select ` → type names.
	afterSelect := "select "
	tl := completionLabels(lsp.QueryCompletion(afterSelect, len(afterSelect), schema))
	if !tl["Project"] || !tl["User"] {
		t.Errorf("select completion should list types; got %v", keys(tl))
	}

	// After a param `<` → builtin/annotation types (not object types).
	ann := "multi select Project { id } limit $n<"
	al := completionLabels(lsp.QueryCompletion(ann, len(ann), schema))
	if !al["int32"] || al["Project"] {
		t.Errorf("annotation completion should list builtins, not object types; got %v", keys(al))
	}

	// Operand position — after `filter`, after either arm of a boolean chain, and
	// after `order by` — offers `.field` paths, since that is what an operand takes.
	for _, prefix := range []string{
		"multi select Project { id } filter ",
		"multi select Project { id } filter .name = $n and ",
		"multi select Project { id } filter .name = $n or ",
		"multi select Project { id } order by ",
	} {
		ol := completionLabels(lsp.QueryCompletion(prefix, len(prefix), schema))
		for _, want := range []string{".name", ".owner"} {
			if !ol[want] {
				t.Errorf("operand completion after %q missing %q; got %v", prefix, want, keys(ol))
			}
		}
		if ol["select"] {
			t.Errorf("operand completion after %q should not offer statement keywords; got %v", prefix, keys(ol))
		}
	}
}

// The LSP must accept boolean filters — chained and grouped — rather than
// reporting them as parse errors, and must still resolve fields inside them.
func TestQueryDiagnosticsBooleanFilter(t *testing.T) {
	schema := parseSchema(t, lspSchema)

	for _, q := range []string{
		"multi select Project { id } filter .name = $n<str> and .owner = $o<str>;",
		"multi select Project { id } filter (.name = $n<str> or .id = $i<uuid>) and .owner = $o<str>;",
	} {
		if d := lsp.QueryDiagnostics(q, schema); len(d) != 0 {
			t.Errorf("valid boolean filter should have no diagnostics: %q → %+v", q, d)
		}
	}

	// An unknown field inside a chain is still reported.
	d := lsp.QueryDiagnostics("multi select Project { id } filter .name = $n and .nope = $x;", schema)
	if len(d) != 1 || !strings.Contains(d[0].Message, "nope") {
		t.Errorf("expected an unknown-field diagnostic inside a chained filter, got %+v", d)
	}
}

func TestPositionRoundTrip(t *testing.T) {
	text := "café = 1\nsecond line"
	// The '=' sits after a multi-byte 'é'; round-trip its offset.
	off := strings.IndexByte(text, '=')
	pos := lsp.OffsetToPosition(text, off)
	if pos.Line != 0 {
		t.Errorf("expected line 0, got %d", pos.Line)
	}
	if got := lsp.PositionToOffset(text, pos); got != off {
		t.Errorf("round-trip offset mismatch: %d != %d", got, off)
	}
	// Second line start.
	nl := strings.IndexByte(text, '\n')
	p2 := lsp.OffsetToPosition(text, nl+1)
	if p2.Line != 1 || p2.Char != 0 {
		t.Errorf("expected line 1 char 0, got %+v", p2)
	}
}

func completionLabels(items []lsp.CompletionItem) map[string]bool {
	m := map[string]bool{}
	for _, it := range items {
		m[it.Label] = true
	}
	return m
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
