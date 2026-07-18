---
title: Tutorial
description: Build your first Axel project end to end — schema, migrations, queries, and typed code
---

# Tutorial: your first Axel project

This walkthrough takes you from an empty folder to a running app backed by
PostgreSQL — using Axel for the schema, the migrations, the queries, and the
generated client code. By the end you'll have:

- a schema written in **ASL** and applied to a real database as a migration,
- two queries written in **AQL**, and
- typed **TypeScript** and **Go** code generated from them, called from a small program.

We'll build a tiny blog: `User`s who write `Post`s.

::: tip Prefer to read the finished result first?
Every file in this tutorial mirrors the runnable [`examples/basic`](https://github.com/struckchure/axel/tree/main/examples/basic)
project in the repo. Clone it if you'd rather poke at the end state.
:::

---

## Prerequisites

- **The `axel` CLI** — see [Installation](./installation). Verify with `axel version`.
- **A PostgreSQL database.** Any Postgres works; this tutorial uses a throwaway
  one in Docker (below), so you don't touch a real database.
- One of a **Bun** or **Go** toolchain, for the final "run it" step.

There is no `axel init` — an Axel project is just a config file, a schema file,
and a folder of queries that you create yourself. That's what we do in Step 1.

---

## Step 1 — Create the project

Make a folder and lay out these files:

```
blog/
  axel.yaml          # project config
  schema.asl         # your schema (ASL)
  queries/           # your queries (AQL) — we add files here later
```

`axel.yaml` tells Axel where your schema and migrations live, and how to reach
the database:

```yaml [axel.yaml]
database-url: postgres://user:password@localhost:5432/db?sslmode=disable
schema-path: ./schema.asl
migrations-dir: ./migrations
```

When you run any command with `-d .` (short for `--dir .`), Axel discovers this
`axel.yaml` automatically and uses it as the full config. See
[Global flags](./cli#global-flags) for the discovery rules.

::: tip Keep the URL out of the file
You can drop `database-url` from `axel.yaml` and instead export
`DATABASE_URL=postgres://…` in your shell — Axel reads it as the default
connection URL.
:::

---

## Step 2 — Write the schema (ASL)

Put this in `schema.asl`. It defines an abstract `Base` type (shared columns),
then `User`, `Post`, and `Comment` that extend it:

```asl [schema.asl]
abstract type Base {
  required id: uuid {
    default := gen_uuid();
    constraint exclusive;
    constraint pk;
  };
  required created_at: datetime { default := datetime_current(); };
  required updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();
  };
}

type User extending Base {
  required email: str {
    constraint exclusive;
    constraint min_length(10);
    constraint max_length(100);
  };
  name: str { default := 'n/a'; };
  required age: int32;
  required health: int32;
  active: bool { default := true };
}

type Post extending Base {
  required title: str;
  required content: str;
  required link author: User;   # single link → FK column "author"
  multi link likes: User;       # multi link → junction table "post_likes"
}

type Comment extending Base {
  required content: str;
  required link post: Post;
  required link author: User;
}
```

A few things worth noticing, each covered in the [ASL reference](./asl):

- **`abstract type Base`** is never a table on its own; its fields are inlined
  into every type that `extending`s it.
- **`constraint`s** (`exclusive`, `pk`, `min_length`) compile to SQL
  `UNIQUE` / `PRIMARY KEY` / `CHECK` clauses.
- **`required link author: User`** is a one-to-many foreign key; **`multi link
  likes: User`** is a many-to-many that Axel backs with a junction table.

---

## Step 3 — Validate it

Before touching the database, check the schema parses and type-checks:

```sh
axel -d . validate
```

```
schema "schema.asl" is valid (4 types)
```

`validate` never connects to a database, so it's the fastest feedback loop while
you're editing the schema.

---

## Step 4 — Start PostgreSQL

Any Postgres will do. To spin up a disposable one matching the URL in
`axel.yaml`, drop this `docker-compose.yaml` next to it and start it:

```yaml [docker-compose.yaml]
services:
  postgres:
    image: postgres:17-alpine
    environment:
      - POSTGRES_USER=user
      - POSTGRES_PASSWORD=password
      - POSTGRES_DB=db
    ports:
      - 5432:5432
```

```sh
docker compose up -d
```

---

## Step 5 — Generate and apply the first migration

`axel generate` diffs your schema against the last migration and writes a new
one. On a fresh project that diff is "create everything":

```sh
axel -d . generate -n "initial schema"
```

This writes a versioned migration folder:

```
migrations/
  0001/
    up.sql          # forward migration
    down.sql        # rollback
    metadata.json   # version, checksum, schema snapshot
```

Open `migrations/0001/up.sql` to see the SQL Axel produced — abbreviated here:

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE "user" (
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now(),
  "email" TEXT NOT NULL UNIQUE,
  "name" TEXT DEFAULT 'n/a',
  "age" INTEGER NOT NULL,
  "health" INTEGER NOT NULL,
  "active" BOOLEAN DEFAULT true,
  "id" UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE PRIMARY KEY
);

CREATE TABLE "post" (
  "content" TEXT NOT NULL,
  "id" UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE PRIMARY KEY,
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now(),
  "title" TEXT NOT NULL,
  "author" UUID NOT NULL,
  FOREIGN KEY ("author") REFERENCES "user"("id") ON DELETE CASCADE
);

-- plus "post_likes" (junction for multi link likes) and "comment"
```

Apply it:

```sh
axel -d . up
```

Axel records applied migrations in a `_axel_migrations` table it creates on first
run. Check the state any time with:

```sh
axel -d . status
```

```
0001    applied
```

::: tip Rolling back
`axel -d . down 1` reverts the last migration using its `down.sql`. Edit the
schema and re-run `generate` to produce `0002`, and so on.
:::

---

## Step 6 — Write queries (AQL)

Queries live in `.aql` files. Create two under `queries/`.

An **insert** that takes one parameter (`queries/create_user.aql`):

```aql [queries/create_user.aql]
insert User {
  email := $email,
  age := 100,
  health := 100
};
```

A **nested read** — every user with their posts, pulled in a single query
(`queries/list_users_with_post.aql`):

```aql [queries/list_users_with_post.aql]
multi select User {
  id,
  email,
  posts := (select Post { id, title } filter .author.id = User.id)
}
```

The `posts := (…)` shape is the important bit: Axel compiles it to a
`json_agg` sub-select, so related rows come back as a nested JSON array with **no
N+1 queries**. See the [AQL reference](./aql) for filters, ordering, and the
other statement types.

---

## Step 7 — See the compiled SQL (optional)

Before generating code, you can inspect exactly what a query compiles to. `axel
compile` needs no database:

```sh
axel -d . compile --file queries/list_users_with_post.aql
```

```sql
SELECT
  u.id AS id,
  u.email AS email,
  (SELECT json_agg(row_to_json(p_posts_sub)) FROM (SELECT p.id AS id, p.title AS title FROM "post" p WHERE p.author = u.id) p_posts_sub) AS posts
FROM "user" u;
```

Code generation (next) runs this compiler for you, so this step is purely for
seeing under the hood.

---

## Step 8 — Generate typed code

Point `codegen` at the project and pick a generator. It compiles every `.aql`
file and emits a typed client:

::: code-group

```sh [TypeScript]
axel -d . codegen -g ts -o ./gen
```

```sh [Go]
axel -d . codegen -g go -o ./gen --option package=generated
```

:::

You get one models file, one file per query, and a `runner` that ties them
together:

```
gen/
  models.{ts,go}                  # one interface/struct per concrete type
  create_user.{ts,go}             # typed params + row + function
  list_users_with_post.{ts,go}
  runner.{ts,go}                  # Runner with typed query methods
```

`models` is one type per concrete ASL type (abstract `Base` is inlined, not
emitted):

::: code-group

```ts [models.ts]
// Code generated by axel codegen --generator ts. DO NOT EDIT.

export interface User {
  active: boolean | null;
  age: number;
  createdAt: Date;
  email: string;
  health: number;
  id: string;
  name: string | null;
  updatedAt: Date;
}
// … Post, Comment
```

```go [models.go]
// Code generated by axel codegen --generator go. DO NOT EDIT.

package generated

type User struct {
	Active    *bool     `json:"active" db:"active"`
	Age       int32     `json:"age" db:"age"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	Email     string    `json:"email" db:"email"`
	Health    int32     `json:"health" db:"health"`
	ID        string    `json:"id" db:"id"`
	Name      *string   `json:"name" db:"name"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
// … Post, Comment
```

:::

Each query becomes a fully-typed function. Here's the nested read — note how the
`posts` shape produced a nested `Posts` type:

::: code-group

```ts [list_users_with_post.ts]
// Code generated by axel codegen --generator ts. DO NOT EDIT.

import type { DB } from "./runner.ts";

export interface ListUsersWithPostRow {
  id: string;
  email: string;
  posts: ListUsersWithPostRowPosts[];
}

export interface ListUsersWithPostRowPosts {
  id: string;
  title: string;
}

export async function listUsersWithPost(db: DB): Promise<ListUsersWithPostRow[]> {
  const query = `SELECT
  u.id AS id,
  u.email AS email,
  (SELECT json_agg(row_to_json(p_posts_sub)) FROM (SELECT p.id AS id, p.title AS title FROM "post" p WHERE p.author = u.id) p_posts_sub) AS posts
FROM "user" u;`;
  return db.unsafe<ListUsersWithPostRow>(query);
}
```

```go [list_users_with_post.go]
// Code generated by axel codegen --generator go. DO NOT EDIT.

package generated

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ListUsersWithPostRow struct {
	ID    string                      `json:"id" db:"id"`
	Email string                      `json:"email" db:"email"`
	Posts []ListUsersWithPostRowPosts `json:"posts" db:"posts"`
}

type ListUsersWithPostRowPosts struct {
	ID    string `json:"id" db:"id"`
	Title string `json:"title" db:"title"`
}

func ListUsersWithPost(ctx context.Context, db *pgxpool.Pool) ([]ListUsersWithPostRow, error) {
	const query = `SELECT
  u.id AS id,
  u.email AS email,
  (SELECT json_agg(row_to_json(p_posts_sub)) FROM (SELECT p.id AS id, p.title AS title FROM "post" p WHERE p.author = u.id) p_posts_sub) AS posts
FROM "user" u;`
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[ListUsersWithPostRow])
}
```

:::

The full generator options — the TypeScript `client` (`bun` vs `pg`), the Go
`package`, enum handling, and the `@name` / `@request` / `@response` directives —
are in the [Code Generation guide](./codegen).

---

## Step 9 — Use it in your app

The generated `Runner` exposes every query as a typed method under `query`
(TypeScript) / `Query` (Go). Wire it to a database connection and call them.

::: code-group

```ts [app.ts — Bun]
import { SQL } from "bun";
import { Runner } from "./gen/runner.ts";

const sql = new SQL({
  url: "postgres://user:password@localhost:5432/db?sslmode=disable",
});
const runner = new Runner(sql);

// INSERT — typed params in, typed row out
const alice = await runner.query.createUser({ email: "alice@example.com" });
console.log("created", alice?.id);

// Nested read — users with their posts, in a single round-trip
const users = await runner.query.listUsersWithPost();
for (const u of users) {
  console.log(u.email, u.posts.map((p) => p.title));
}
```

```go [main.go — pgx]
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	generated "yourmodule/gen"
)

func main() {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, "postgres://user:password@localhost:5432/db?sslmode=disable")
	if err != nil {
		log.Fatalln(err)
	}
	defer db.Close()

	runner := generated.NewRunner(db)

	// INSERT — typed params in, typed row out
	alice, err := runner.Query.CreateUser(ctx, generated.CreateUserParams{Email: "alice@example.com"})
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println("created", alice.ID)

	// Nested read — users with their posts, in a single round-trip
	users, err := runner.Query.ListUsersWithPost(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	for _, u := range users {
		fmt.Println(u.Email, len(u.Posts))
	}
}
```

:::

Run it:

::: code-group

```sh [TypeScript]
bun run app.ts
```

```sh [Go]
go run .
```

:::

::: tip Bun is the default TS client
The generated TypeScript targets Bun's `SQL` class out of the box. For
[node-postgres](https://node-postgres.com) instead, regenerate with
`--option client=pg` and pass a `Pool` to the `Runner`.
:::

---

## Recap

You went from an empty folder to a working, type-safe data layer:

1. **`axel.yaml` + `schema.asl`** — declared the project and its types.
2. **`axel validate`** — checked the schema with no database.
3. **`axel generate` + `axel up`** — turned the schema into migration SQL and applied it.
4. **`.aql` files** — wrote queries, including a nested shape that avoids N+1.
5. **`axel codegen`** — got typed TypeScript and Go clients.
6. Called the generated `Runner` from a real program.

The whole loop — edit schema → `generate` → `up`, edit queries → `codegen` — is
what you repeat as the project grows.

## Next steps

- [Schema Language (ASL)](./asl) — enums, computed fields, indexes, all constraints.
- [Query Language (AQL)](./aql) — filters, ordering, `insert`/`update`/`delete`, operators.
- [Code Generation](./codegen) — generator options, directives, and writing your own generator.
- [CLI Reference](./cli) — every command and flag.
- [Editor setup](./editors) — syntax highlighting and the language server.
