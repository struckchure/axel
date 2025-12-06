package axel

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}

	return strings.ToLower(result.String())
}

func getNodeText(node *tree_sitter.Node, code []byte) string {
	return string(code[node.StartByte():node.EndByte()])
}
