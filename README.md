<p align="center">
  <img src="docs/public/logo.svg" alt="Axel logo" width="96" height="96">
</p>

<h1 align="center">Axel</h1>

<p align="center">
  <a href="https://github.com/struckchure/axel/actions/workflows/build-and-release.yaml"><img src="https://github.com/struckchure/axel/actions/workflows/build-and-release.yaml/badge.svg?branch=main" alt="Tests"></a>
</p>

<p align="center">Schema and query languages for PostgreSQL, compiled to SQL.</p>

**Axel** is a schema and query language tool for PostgreSQL. You define your data model in **ASL** (Axel Schema Language) and write queries in **AQL** (Axel Query Language). Axel compiles both to SQL — migrations from ASL, parameterized query strings from AQL.

Axel never wraps a driver or executes queries on your behalf. It generates SQL; you run it however you like.

---

> [!NOTE]
> **Disclaimer:** The code in this repository is majorly AI-generated. Review it carefully before relying on it in production.

---

## Why Axel?

Modern applications rely on PostgreSQL for its power, performance, and advanced capabilities. Yet the tools we use to interact with PostgreSQL often fight against it:

* **Framework-coupled ORMs (Django, Rails Active Record):** Deeply embedded inside specific runtimes. They abstract away the relational model so heavily that complex queries stop feeling like SQL and become impossible to optimize without dropping to raw text.
* **Fragile relationship models (TypeORM, Sequelize):** Convoluted entity decorators, confusing ownership models, and endless mental gymnastics just to model simple one-to-many or many-to-many relationships.
* **TypeScript-first query builders (Drizzle, Kysely):** Great for type inference, but relationship definitions and relational queries often feel estranged and bolted-on.
* **Black-box runtime ORMs (Prisma 6 vs. Prisma 7):** Prisma 6 relied on a heavy Rust binary sidecar (causing high latency, large bundles, and serverless cold starts). Prisma 7 migrated to TypeScript/WASM to fix binary issues, but locked Prisma strictly to the JS/TS ecosystem. In both versions, it remains a heavy runtime layer that struggles with native upsert conflict handling (`ON CONFLICT DO UPDATE`), cannot declaratively manage PostgreSQL extensions/triggers, and incurs query execution overhead.
* **Neglected PostgreSQL features:** Good luck defining native PostgreSQL extensions (`pgvector`, `pg_trgm`, `postgis`, `citext`), row-level security (RLS) policies, triggers, custom functions, partial indexes, and conditional aggregates cleanly across any of them.
* **Enterprise grade:** Traditional ORMs struggle to deliver clean, predictable SQL that database administrators and high-throughput systems demand.

### The Axel Approach

Axel rethinks database tooling by separating **data modeling and query compilation** from **runtime execution**:

1. **Compiler, Not a Runtime ORM:** Axel does not wrap a driver, maintain a connection pool, or execute queries behind your back. It compiles `.asl` schemas into deterministic migration files and `.aql` queries into pure, optimal PostgreSQL queries. You run the SQL using your language's native driver (`pgx`, `node-postgres`, `sqlx`, etc.).
2. **First-Class Relational Modeling:** Define single links (`link author: User`) and many-to-many relationships (`multi link members: User`) directly in ASL. Axel automatically creates and manages junction tables and foreign keys.
3. **Zero N+1 Query Aggregation:** Fetch deeply nested relational shapes in a single query. Axel compiles nested shapes directly to PostgreSQL's native `json_agg` in one scan — no N+1 query problem, no multiple roundtrips.
4. **Native PostgreSQL First:** Built-in syntax for PostgreSQL extensions (`extension pgcrypto;`), triggers (`trigger ... do (...)`), row-level security (`policy ... using (...)`), conditional aggregates (`FILTER (WHERE ...)`), top-level CTEs (`with (...)`), and upserts (`unless conflict on ... else (...)`).
5. **Universal Language Support:** Because Axel compiles to pure SQL, it works with any programming language. Axel provides first-party code generators for Go and TypeScript, and you can write a generator for any other language in Go (or as an external plugin). If your language generator is missing, [file an issue](https://github.com/struckchure/axel/issues) to request it!
6. **Integrated Tooling:** Language server (LSP) for VSCode and Zed, and an interactive database studio (`axel studio`).

---

## How it works

[X](https://x.com/struckchure/status/2079922001922122004) | [YouTube](https://www.youtube.com/watch?v=79oG_GC8d2A)

```
schema.asl  ──► axel diff ──► migration.sql ──► axel up ──► PostgreSQL
query.aql   ──► axel compile  ──► parameterized SQL (you execute this)
```

---

## Schema language (ASL)

Define types, links, constraints, and indexes in `.asl` files:

```asl
abstract type Base {
  required id: uuid { default := gen_uuid(); constraint pk; };
  required created_at: datetime { default := datetime_current(); };
}

type User extending Base {
  required email: str { constraint exclusive; };
  name: str;
  required age: int32;
  active: bool { default := true };
}

type Post extending Base {
  required title: str;
  required link author: User;   # adds author_id FK column
  multi link likes: User;       # creates post_likes junction table
}
```

Run `axel diff -n "initial schema"` to diff against the last migration and produce a `.sql` file. Run `axel up` to apply it.

Full reference: [docs/asl.md](docs/asl.md)

---

## Query language (AQL)

Write queries in `.aql` files and compile them to parameterized SQL:

```aql
multi select User {
  id,
  email,
  posts: {
    id,
    title
  }
}
filter .active = true and .age >= $min_age
order by .created_at desc
limit $limit;
```

```sql
-- $1: min_age
-- $2: limit
SELECT
  u.id AS id,
  u.email AS email,
  (SELECT COALESCE(json_agg(row_to_json(p_posts_sub)), '[]')
   FROM (...) p_posts_sub) AS posts
FROM "user" u
WHERE u.active = true AND u.age >= $1
ORDER BY u.created_at DESC
LIMIT $2;
```

Nested shapes compile to a single query using PostgreSQL's `json_agg` — no N+1.

AQL supports SELECT, INSERT, UPDATE, and DELETE:

```aql
insert User { email := $email, name := $name, age := $age };

update User filter .id = $id set { name := $name };

delete User filter .id = $id;

select count(User filter .active = true);
```

Full reference: [docs/aql.md](docs/aql.md)

---

## CLI

### Setup

Point axel at your project directory and it discovers the config automatically:

```sh
axel -d ./myproject validate
axel -d ./myproject compile --aql 'select User { id, email };'
axel -d ./myproject up
```

Discovery order inside `--dir`:

1. `axel.yaml` — loaded as the full config if found
2. `schema.asl` — used as the schema if no `axel.yaml`
3. `default.asl` — fallback schema name

Or use an explicit config file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/struckchure/axel/main/schema.json
database-url: postgres://user:pass@localhost:5432/mydb
schema-path: ./schema.asl
migrations-dir: ./migrations
rel-load-strategy: query # query | join

codegen:
  generator: go # go | ts
  out-dir: ./db/generated
  queries:
    - ./queries/*.aql
```

```sh
axel -c axel.yaml <command>
```

### Commands

| Command         | Description                                |
| --------------- | ------------------------------------------ |
| `axel init`     | Scaffold a new Axel project (config, schema, migrations) |
| `axel validate` | Parse and validate a `.asl` schema file    |
| `axel compile`  | Compile an AQL query to parameterized SQL  |
| `axel codegen`  | Generate type-safe Go or TypeScript code   |
| `axel diff`     | Diff schema and write a new migration file |
| `axel up`       | Apply all pending migrations               |
| `axel down <n>` | Roll back the last N migrations            |
| `axel status`   | Show migration state                       |

```sh
# Initialize project
axel init

# Validate schema
axel -d . validate

# Compile a query (use single quotes to prevent shell $expansion)
axel -d . compile --aql 'select User { id, email } filter .id = $id;'
axel -d . compile --file queries/get_user.aql --out queries/get_user.sql

# Migrations
axel -d . diff -n "add posts table"
axel -d . up
axel -d . down 1
axel -d . status
```

Full reference: [docs/cli.md](docs/cli.md)

---

## Typical workflow

```sh
# 1. Write your schema
vim schema.asl

# 2. Validate it
axel -d . validate

# 3. Generate a migration
axel -d . diff -n "initial schema"

# 4. Apply it
axel -d . up

# 5. Write queries
vim queries/get_users.aql

# 6. Compile to SQL
axel -d . compile --file queries/get_users.aql

# 7. Execute the SQL in your application however you like
```

---

## Editor support

A Zed extension provides syntax highlighting for `.asl` and `.aql` files.

To install it, open the command palette in Zed, run **`zed: install dev extension`**, and select the [`tools/zed`](tools/zed) directory.

See [tools/zed/README.md](tools/zed/README.md) for details.

---

## Documentation

- [docs/asl.md](docs/asl.md) — Axel Schema Language reference
- [docs/aql.md](docs/aql.md) — Axel Query Language reference
- [docs/cli.md](docs/cli.md) — CLI commands and flags

---

## License

See [LICENSE](./LICENSE) for details.
