package main

import (
	"fmt"

	"github.com/spf13/cobra"
	axel "github.com/struckchure/axel/core"
)

func migrateGenerate(name string) error {
	generator := axel.NewMigrationGenerator(manager)
	return generator.GenerateMigration(name)
}

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Diff the schema against the last migration and write a new migration",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		if err := migrateGenerate(name); err != nil {
			fmt.Println(err)
		}
	},
}

// generateCmd is the former name of `diff`, kept as a deprecated alias so
// existing scripts keep working. Cobra prints a deprecation notice to stderr.
var generateCmd = &cobra.Command{
	Use:        "generate",
	Short:      "Deprecated alias for \"diff\"",
	Deprecated: "use \"axel diff\" instead",
	Run:        diffCmd.Run,
}

func init() {
	diffCmd.Flags().StringP("name", "n", "", "Migration Name")
	generateCmd.Flags().StringP("name", "n", "", "Migration Name")
	RootCmd.AddCommand(diffCmd)
	RootCmd.AddCommand(generateCmd)
}
