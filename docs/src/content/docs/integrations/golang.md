---
title: "Go — Integrations"
description: "End to end — from axel init to a typed Go client"
---

# Go

A full walkthrough: scaffold a project, design a schema, apply migrations, then
generate and use Axel's typed Go client. See [Code Generation](/codegen) for the
generator reference and [Schema Language](/asl/) for ASL details.

## 1. Scaffold the project

```sh
axel init
```

This writes a starter project:

```
axel.yaml            # config: schema-path, migrations-dir, database-url
axel/schema.asl      # a Base abstract type + a starter User
axel/migrations/     # empty; migrations land here
```

`axel.yaml` points the database URL at an env var so secrets stay out of the file:

```yaml
schema-path: axel/schema.asl
migrations-dir: axel/migrations
database-url: $env.DATABASE_URL
```

Set that to your Postgres — local, [Supabase](/integrations/supabase), or
[Neon](/integrations/neon):

```sh
export DATABASE_URL='postgresql://user:pass@localhost:5432/app?sslmode=disable'
```

## 2. Design the schema

The starter `axel/schema.asl` already defines a reusable `Base` (uuid primary key
+ `created_at` / `updated_at`) and a `User`. Add a `Post` linked to `User`:

```asl
type Post extends Base {
  required title: str;
  content: str;
  required author: User;
}
```

Type-check the schema at any time — no database needed:

```sh
axel validate
```

## 3. Diff and apply

Generate a migration from the schema, then apply it to the database:

```sh
axel diff -n init   # writes axel/migrations/0001_init
axel up             # applies pending migrations
```

`axel up` records applied migrations in an `_axel_migrations` table, so it's safe
to re-run. See the [CLI reference](/cli) for `diff` / `up` / `down`.

## 4. Write a query

Queries live in `.aql` files. Create `queries/list_post.aql` and
`queries/get_user.aql`:

```aql
# list_post.aql
multi select Post { id, title, content };
```

```aql
# get_user.aql
select User { id, name, email } filter .id = $id<uuid>;
```

`multi select` returns many rows (`[]ListPostRow`); a plain `select` returns a
single row (`*GetUserRow`).

## 5. Generate the client

```sh
axel codegen -g go -o ./gen --option package=gen
```

Axel auto-discovers every `*.aql` file under the project directory
and emits a `gen/` package (`runner.go`, `models.go`, one file per query). A
query's filename becomes a **PascalCase** method: `list_post.aql` →
`runner.Query.ListPost(ctx)`. The package is named `generated` unless you override
it with `--option package=...`. (Or name files explicitly: `-q 'queries/*.aql'`.)

The generated package imports Axel's runtime, so add Axel and `pgx` to your module:

```sh
go get github.com/struckchure/axel github.com/jackc/pgx/v5
```

## 6. Connect and call

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
    gen "github.com/you/app/gen"
)

func main() {
    ctx := context.Background()
    db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    runner := gen.NewRunner(db)

    posts, err := runner.Query.ListPost(ctx)                          // []ListPostRow
    user, err := runner.Query.GetUser(ctx, gen.GetUserParams{ID: id}) // *GetUserRow
    _ = posts
    _ = user
}
```

Params and rows are generated structs (`GetUserParams`, `ListPostRow`) with `db`
and `json` tags; `datetime` maps to `time.Time`, nullable columns to pointers
(`*string`), and single-row queries return `*XxxRow`.

### Dynamic escape hatch

For ad-hoc queries that aren't in a `.aql` file, `runner.Run` executes raw AQL:

```go
rows, err := runner.Run(ctx,
    `select Post { id } filter .author.id = $author`,
    map[string]any{"author": author},
)
```

:::tip[Regenerate (`axel codegen -g go -o ./gen`) whenever you change a query or the]
schema, so the types stay in sync. At runtime the client only needs a Postgres
connection string, so point it at your provider's pooled endpoint and keep
[migrations](/cli) on the direct one.
:::
