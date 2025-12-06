package axel

import (
	"fmt"

	"github.com/samber/lo"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func formatIdentifier(s string) string {
	return fmt.Sprintf(`"%s"`, lo.SnakeCase(s))
}

func getNodeText(node *tree_sitter.Node, code []byte) string {
	return string(code[node.StartByte():node.EndByte()])
}
