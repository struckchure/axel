package main

import (
	"fmt"
	"os"

	tree_sitter_axel "github.com/struckchure/axel/bindings/go"
	axel "github.com/struckchure/axel/core"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func main() {
	code, _ := os.ReadFile("./examples/sdl/default.axel")

	parser := tree_sitter.NewParser()
	defer parser.Close()
	err := parser.SetLanguage(tree_sitter.NewLanguage(tree_sitter_axel.Language()))
	if err != nil {
		fmt.Printf("ERROR: Failed to set language: %v\n", err)
		return
	}

	tree := parser.Parse(code, nil)
	defer tree.Close()

	root := tree.RootNode()

	// Extract models from AST
	models := extractModels(root, code)

	// Resolve OnTarget types
	axel.ResolveOnTargetTypes(models)

	// Generate SQL
	sql := axel.GenerateSQL(models)
	fmt.Println(sql)
}

func extractModels(root *tree_sitter.Node, code []byte) []axel.Model {
	var models []axel.Model

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(uint(i))

		switch child.Kind() {
		case "model", "abstract_model":
			model := axel.ParseModel(child, code)
			models = append(models, model)
		}
	}

	return models
}
