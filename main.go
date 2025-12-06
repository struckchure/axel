package main

import (
	"fmt"

	"github.com/struckchure/axel/cmd"
)

func main() {
	// parser := tree_sitter.NewParser()
	// lang := tree_sitter.NewLanguage(tree_sitter_axel.Language())

	// manager, err := axel.NewMigrationManager(&axel.MigrationConfig{
	// 	MigrationsDir: "./examples/sdl/migrations",
	// 	SchemaPath:    "./examples/sdl/default.axel",
	// 	DatabaseURL:   "postgres://user:password@localhost:5432/db",
	// })
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }

	// generator := axel.NewMigrationGenerator(manager, parser, lang)
	// err = generator.GenerateMigration("init")
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }

	err := cmd.RootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		return
	}
}
