package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	tree_sitter_axel "github.com/struckchure/axel/bindings/go"
	axel "github.com/struckchure/axel/core"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// migrateGenerate generates a new migration from schema changes
func migrateGenerate(name string) error {
	// Setup tree-sitter parser
	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_axel.Language())

	// Create generator
	generator := axel.NewMigrationGenerator(manager, parser, lang)

	// Generate migration
	if err := generator.GenerateMigration(name); err != nil {
		return err
	}

	return nil
}

var generateCmd = &cobra.Command{
	Use: "generate",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		err := migrateGenerate(name)
		if err != nil {
			fmt.Println(err)
		}
	},
}

func init() {
	generateCmd.Flags().StringP("name", "n", "", "Migration Name")

	RootCmd.AddCommand(generateCmd)
}
