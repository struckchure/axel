package lsp

import (
	"strings"
	"testing"
)

func TestQueryHoverDirectives(t *testing.T) {
	schema := schemaFor(t, enumSchema)

	text := "@rel_load_strategy join\nselect Job { title };"
	// Hover over "rel_load_strategy"
	offset := strings.Index(text, "rel_load_strategy")
	h := QueryHover(text, offset, schema)
	if h == nil {
		t.Fatal("expected hover for rel_load_strategy, got nil")
	}
	if !strings.Contains(h.Contents, "@rel_load_strategy") {
		t.Errorf("hover contents missing directive name:\n%s", h.Contents)
	}

	// Hover over "join"
	offsetJoin := strings.Index(text, "join")
	hJoin := QueryHover(text, offsetJoin, schema)
	if hJoin == nil {
		t.Fatal("expected hover for join, got nil")
	}
	if !strings.Contains(hJoin.Contents, "LEFT JOIN LATERAL") {
		t.Errorf("hover contents missing strategy description:\n%s", hJoin.Contents)
	}
}
