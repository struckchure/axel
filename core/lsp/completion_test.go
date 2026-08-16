package lsp

import (
	"testing"

	"github.com/struckchure/axel/core/asl"
)

func schemaFor(t *testing.T, src string) *asl.SchemaIR {
	t.Helper()
	sf, err := asl.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return ir
}

const enumSchema = `
enum QueueStatus { Pending, Active, Done }
type Job {
  required title: str;
  required status: QueueStatus;
}`

// labels returns the completion labels as a set for order-independent assertions.
func labels(items []CompletionItem) map[string]CompletionItem {
	m := make(map[string]CompletionItem, len(items))
	for _, it := range items {
		m[it.Label] = it
	}
	return m
}

func TestQueryCompletionEnumMembersAfterEnumDot(t *testing.T) {
	schema := schemaFor(t, enumSchema)
	text := `select Job filter .status = QueueStatus.`
	got := labels(QueryCompletion(text, len(text), schema))
	for _, want := range []string{"Pending", "Active", "Done"} {
		it, ok := got[want]
		if !ok {
			t.Errorf("missing enum member %q; got %v", want, keys(got))
			continue
		}
		if it.Kind != CompletionKindEnumMember {
			t.Errorf("%q kind = %d, want %d", want, it.Kind, CompletionKindEnumMember)
		}
	}
	// It should not fall back to object-field completion.
	if _, ok := got["title"]; ok {
		t.Errorf("enum member context should not offer object fields: %v", keys(got))
	}
}

func TestQueryCompletionEnumValuesOnComparisonRHS(t *testing.T) {
	schema := schemaFor(t, enumSchema)
	text := `select Job filter .status = `
	got := labels(QueryCompletion(text, len(text), schema))
	// The RHS of `= ` where the LHS is enum-typed offers qualified members.
	for _, want := range []string{"QueueStatus.Pending", "QueueStatus.Active", "QueueStatus.Done"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing qualified enum value %q; got %v", want, keys(got))
		}
	}
}

func TestQueryCompletionNonEnumDotStillGivesFields(t *testing.T) {
	schema := schemaFor(t, enumSchema)
	text := `select Job { .`
	got := labels(QueryCompletion(text, len(text), schema))
	if _, ok := got["title"]; !ok {
		t.Errorf("expected object field 'title' after '.', got %v", keys(got))
	}
}

func TestSchemaCompletionEnumMembersInDefault(t *testing.T) {
	schema := schemaFor(t, enumSchema)
	text := enumSchema + "\ntype T { required s: QueueStatus { default := QueueStatus."
	got := labels(SchemaCompletion(text, len(text), schema))
	for _, want := range []string{"Pending", "Active", "Done"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing enum member %q in default context; got %v", want, keys(got))
		}
	}
}

func TestQueryCompletionDirectives(t *testing.T) {
	schema := schemaFor(t, enumSchema)
	text := `@`
	got := labels(QueryCompletion(text, len(text), schema))
	for _, want := range []string{"name", "request", "response", "rel_load_strategy"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing directive %q after @; got %v", want, keys(got))
		}
	}
}

func TestQueryCompletionRelLoadStrategyOptions(t *testing.T) {
	schema := schemaFor(t, enumSchema)
	text := `@rel_load_strategy `
	got := labels(QueryCompletion(text, len(text), schema))
	for _, want := range []string{"query", "join"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing strategy option %q; got %v", want, keys(got))
		}
	}
}

func keys(m map[string]CompletionItem) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

