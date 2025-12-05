package tree_sitter_axel_test

import (
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_axel "github.com/struckchure/axel/bindings/go"
)

func TestCanLoadGrammar(t *testing.T) {
	language := tree_sitter.NewLanguage(tree_sitter_axel.Language())
	if language == nil {
		t.Errorf("Error loading Axel grammar")
	}
}
