# Axel Code Generation Reference

Axel generates typed database clients from your schema (`.asl`) and queries (`.aql`).

```sh
axel codegen -g go -o ./gen --option package=gen
axel codegen -g ts -o ./gen
axel codegen --plugin ./my-generator -o ./gen
```

---

## 1. Go Generator (`-g go`)

Generates a Go package targeting [`pgx/v5`](https://github.com/jackc/pgx).

### CLI Options

| Option | Default | Description |
|---|---|---|
| `package` | `generated` | Package name for all generated `.go` files |

### Generated Files

- `models.go` — Structs for all non-abstract ASL types and enum constants.
- `<query_name>.go` — Typed query function, parameter struct, and row struct per `.aql` query.
- `runner.go` — `Runner` and `Queries` structs, `DBTX` interface, `NewRunner`, `NewQueries`, `WithDB`, and global setters.

### Usage

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
    gen "myapp/gen"
)

func main() {
    ctx := context.Background()
    db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    runner := gen.NewRunner(db)

    // Call compiled queries via runner.Query
    posts, err := runner.Query.ListPost(ctx)
    user, err := runner.Query.GetUser(ctx, gen.GetUserParams{ID: "..."})

    // Run dynamic AQL at runtime
    rows, err := runner.Run(ctx, `select User { id, email } filter .id = $id`, map[string]any{"id": "..."})
}
```

### Transactions & Custom Connections (`WithDB`, `NewQueries`)

Pass any `DBTX` (`*pgxpool.Pool` or `pgx.Tx`) to `WithDB()` or `NewQueries()`:

```go
tx, err := db.Begin(ctx)
if err != nil {
    return err
}
defer tx.Rollback(ctx)

// Rebind queries to the transaction:
q := runner.WithDB(tx)
// or: q := runner.Query.WithDB(tx)
// or: q := gen.NewQueries(tx)

user, err := q.CreateUser(ctx, userParams)
if err != nil {
    return err
}
doc, err := q.CreateDoc(ctx, docParams)
if err != nil {
    return err
}

return tx.Commit(ctx)
```

Standalone query functions also accept `DBTX`:

```go
user, err := gen.GetUser(ctx, tx, gen.GetUserParams{ID: "..."})
```

---

## 2. TypeScript Generator (`-g ts`)

Generates TypeScript client files targeting either **Bun SQL** (default) or **node-postgres** (`pg`).

### CLI Options

| Option | Default | Description |
|---|---|---|
| `client` | `bun` | Target driver: `bun` (uses Bun's `SQL`) or `pg` (uses `pg.Pool`) |

### Generated Files

- `models.ts` — Type interfaces and enums for all concrete ASL types.
- `<query_name>.ts` — Typed async query function, params interface, and row interface.
- `runner.ts` — `Runner` and `Queries` classes, `withDb`, fluent query builders (`select`, `insert`, `update`), and global setters.

### Usage (Bun)

```ts
import { SQL } from "bun";
import { Runner } from "./gen/runner";

const db = new SQL({ url: process.env.DATABASE_URL });
const runner = new Runner(db);

// Compiled queries
const posts = await runner.query.listPost();
const user = await runner.query.getUser({ id: "..." });
```

### Usage (node-postgres / pg)

Generate with `-g ts --option client=pg`:

```ts
import { Pool } from "pg";
import { Runner } from "./gen/runner";

const pool = new Pool({ connectionString: process.env.DATABASE_URL });
const runner = new Runner(pool);
```

### Transactions & Custom Connections (`withDb`)

Use `withDb(db)` on `Runner` or `Queries` (direct call or scoped callback):

```ts
// Direct call:
const q = runner.withDb(tx);
const doc = await q.createDoc(params);

// Scoped callback:
await runner.withDb(tx, async (q) => {
  const user = await q.createUser(userParams);
  return q.createDoc({ ...docParams, authorId: user.id });
});
```

Standalone query functions also accept `db`:

```ts
import { getUser } from "./gen/get_user.ts";
const user = await getUser(tx, { id: "..." });
```

---

## 3. Globals and Row-Level Security

When the schema defines `global <name>: <type>;`, generators emit scoped transaction setters:

### Go:
```go
err := runner.WithCurrentUser(ctx, userID, func(q *gen.Queries) error {
    return q.CreateDoc(ctx, docParams)
})

// Or via functional options on standalone functions:
doc, err := gen.CreateDoc(ctx, db, docParams, gen.WithCurrentUser(userID))
```

### TypeScript:
```ts
await runner.withCurrentUser(userId, async (q) => {
    return q.createDoc(docParams);
});

// Or via options object on standalone functions:
const doc = await createDoc(db, docParams, { currentUser: userId });
```
