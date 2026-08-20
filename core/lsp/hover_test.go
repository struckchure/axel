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

func TestQueryHoverFunctions(t *testing.T) {
	schema := schemaFor(t, funcSchema)
	text := "select User filter random_string(10);"
	offset := strings.Index(text, "random_string")
	h := QueryHover(text, offset, schema)
	if h == nil {
		t.Fatal("expected hover for function random_string, got nil")
	}
	if !strings.Contains(h.Contents, "function random_string(len: int32) -> str") {
		t.Errorf("hover contents missing function signature:\n%s", h.Contents)
	}
}

func TestSchemaHoverFunctionInDefault(t *testing.T) {
	schemaText := `
function random_hex(len: int32) -> str { return 'hex'; };

scalar type Code extends str {
  constraint min_length(6);
  constraint max_length(6);
  default := random_hex(6);
}
`
	schema := schemaFor(t, schemaText)
	offset := strings.Index(schemaText, "random_hex(6)")
	h := SchemaHover(schemaText, offset, schema)
	if h == nil {
		t.Fatal("expected hover for function random_hex inside default, got nil")
	}
	if !strings.Contains(h.Contents, "function random_hex(len: int32) -> str") {
		t.Errorf("unexpected hover contents:\n%s", h.Contents)
	}
}


