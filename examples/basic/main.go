package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	generated "github.com/struckchure/axel/examples/basic/gen"
)

func main() {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, "postgres://user:password@localhost:5432/db?sslmode=disable")
	if err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	r := generated.NewRunner(db)

	// query := `#aql
	// insert Post {
	// 	title := $title,
	// 	content := $content,
	// 	author := (insert User { email := $email, name := $name, age := 100, health := 100 })
	// };
	// `

	// _, err = r.Run(ctx, query, map[string]any{
	// 	"title":   "Two",
	// 	"content": "One Two Three 4",
	// 	"email":   "user-2@mail.com",
	// 	"name":    "User Two",
	// })
	// if err != nil {
	// 	log.Fatalln(err)
	// }

	users, err := r.Run(
		ctx,
		`
		select User {
			id,
			email,
			posts := (select Post { id, title } filter .author.id = User.id)
		}
		`,
		map[string]any{},
	)
	if err != nil {
		log.Fatalln(err)
	}
	for _, user := range users {
		for idx, post := range user["posts"].([]any) {
			p := post.(map[string]any)
			fmt.Println(idx, " - ", p["id"], p["title"])
		}
	}
}
