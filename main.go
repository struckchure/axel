package main

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/struckchure/axel/clients"
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

	// err := cmd.RootCmd.Execute()
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }

	db, err := sqlx.Connect(
		"postgres",
		"postgres://user:password@localhost:5432/db?sslmode=disable",
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx := context.Background()

	u := new(clients.User)

	user, err := u.Query(db).Where(
		u.EmailEq("john@mail.com"),
		u.NameEq("NULL"),
	).First(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%#v\n", user)

	users, err := u.Query(db).Where().All(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%#v\n", users)
}
