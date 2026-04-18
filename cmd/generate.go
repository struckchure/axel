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

var generateCmd = &cobra.Command{
	Use: "generate",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		if err := migrateGenerate(name); err != nil {
			fmt.Println(err)
		}
	},
}

func init() {
	generateCmd.Flags().StringP("name", "n", "", "Migration Name")
	RootCmd.AddCommand(generateCmd)
}
