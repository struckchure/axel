package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"

	generated "github.com/struckchure/axel/examples/basic/gen"
)

func main() {
	ctx := context.Background()
	db, err := sql.Open("postgres", "postgres://user:password@localhost:5432/db?sslmode=disable")
	if err != nil {
		log.Fatalln(err)
	}
	r := generated.NewRunner(db)

	query := `#aql
	insert Post {
		title := $title,
		content := $content,
		author := (insert User { email := $email, name := $name, age := 100, health := 100 })
	};
	`

	_, err = r.Run(ctx, query, map[string]any{
		"title":   "Two",
		"content": "One Two Three 4",
		"email":   "user-2@mail.com",
		"name":    "User Two",
	})
	if err != nil {
		log.Fatalln(err)
	}

	posts, err := r.ListPost(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	for _, post := range posts {
		fmt.Println(post.ID, post.Title)
	}
}
