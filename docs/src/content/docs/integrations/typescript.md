---
title: "TypeScript — Integrations"
description: "End to end — from axel init to a typed TypeScript client"
---

# TypeScript

A full walkthrough: scaffold a project, design a schema, apply migrations, then
generate and use Axel's typed TypeScript client. See [Code Generation](/codegen)
for the generator reference and [Schema Language](/asl/) for ASL details.

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

`multi select` returns many rows (`ListPostRow[]`); a plain `select` returns a
single row (`GetUserRow | null`).

## 5. Generate the client

```sh
axel codegen -g ts -o ./gen
```

Axel auto-discovers every `*.aql` file under the project directory
and emits a `gen/` folder (`runner.ts`, `models.ts`, one file per query, and an
`index.ts` barrel re-exporting all of them). A query's
filename becomes a **camelCase** method: `list_post.aql` →
`runner.query.listPost()`. (Or name files explicitly: `-q 'queries/*.aql'`.)

The barrel lets you import from the output directory itself rather than reaching
into individual files:

```ts
import { Runner, listPost, type User } from "./gen";
```

The default target is Bun's built-in `SQL`. For node-postgres, add
`--option client=pg` and pass a `Pool` instead.

## 6. Connect and call

```ts
import { SQL } from "bun";
import { Runner } from "./gen/runner";

const db = new SQL({ url: process.env.DATABASE_URL });
const runner = new Runner(db);

const posts = await runner.query.listPost();     // ListPostRow[]
const user = await runner.query.getUser({ id }); // GetUserRow | null
```

Params and rows are generated interfaces (`GetUserParams`, `ListPostRow`), with
`datetime` mapped to `Date`, nullable columns to `T | null`, and camelCase field
names (`createdAt`).

```ts
import { Pool } from "pg"; // bun add pg @types/pg
import { Runner } from "./gen/runner";

const runner = new Runner(new Pool({ connectionString: process.env.DATABASE_URL }));
```

### Transactions

To run queries inside a transaction, use `runner.withDb(tx)`:

```ts
// Direct call on transaction handle:
const q = runner.withDb(tx);
const user = await q.getUser({ id });

// Or scoped callback style:
await runner.withDb(tx, async (q) => {
  const user = await q.createUser(userParams);
  return q.createPost({ ...postParams, authorId: user.id });
});
```

## 7. Ad-hoc queries with the builder

Beyond the generated per-file methods, `runner.select()` and `runner.insert()`
give you a **typed fluent builder** for queries you don't want to commit to a
`.aql` file. The shape argument drives the inferred return type — no codegen step:

```ts
const users = await runner
  .select("User", { id: true, email: true, name: true })
  .where("active", "=", true)
  .and("age", ">=", 18)
  .or("email", "=", "admin@example.com")
  .all();
// users: Array<{ id: string; email: string; name: string | null }>
```

`.and()` / `.or()` are only available after `.where()`. Use `.all()` for many rows
or `.one()` for a single row (`… | null`):

```ts
const user = await runner
  .select("User", { id: true, email: true })
  .where("id", "=", id)
  .one(); // { id: string; email: string } | null
```

Nest a builder as a shape value to pull related rows as a JSON array in one query.
A backtick string is a **correlated reference** to the outer row:

```ts
const authors = await runner
  .select("User", {
    id: true,
    name: true,
    posts: runner
      .select("Post", { title: true })
      .where("authorId", "=", "`User.id`"), // outer-query reference
  })
  .all();
```

Select a **multi link** by naming it in the shape. It is fetched as a correlated
JSON array over the link's junction table — no join or correlated filter to write
by hand, and an empty relation comes back as `[]` rather than `null`:

```ts
const vendors = await runner
  .select("Vendor", { id: true, name: true, members: true })
  .all();
// vendors: Array<{ id: string; name: string; members: User[] }>
```

A multi link is a relation, not a column, so it is selectable but never insertable —
`runner.insert("Vendor", …)` will not accept a `members` key. Use an AQL
[delta assignment](/aql/update/links#multi-links) to modify the junction. Sub-shapes
on a link (`{ members: { id: true } }`) are an AQL-only feature for now; the builder
selects all of the target's columns.

Insert with `runner.insert()`:

```ts
const created = await runner
  .insert("User", { email: "alice@example.com", name: "Alice" })
  .one();
```

See [Code Generation → Fluent select builder](/codegen#fluent-select-builder-runner-select)
for the complete builder API.

### Dynamic escape hatch

For raw AQL that the builder can't express, `runner.run` executes a query string
directly:

```ts
const rows = await runner.run<{ id: string }>(
  `select Post { id } filter .author.id = $author`,
  { author },
);
```

:::tip[Regenerate (`axel codegen -g ts -o ./gen`) whenever you change a query or the]
schema, so the types stay in sync. At runtime the client only needs a Postgres
connection string, so point it at your provider's pooled endpoint and keep
[migrations](/cli) on the direct one.
:::
