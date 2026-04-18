# Axel

**Axel** is a schema and query language tool for PostgreSQL. You define your data model in **ASL** (Axel Schema Language) and write queries in **AQL** (Axel Query Language). Axel compiles both to SQL — migrations from ASL, parameterized query strings from AQL.

Axel never wraps a driver or executes queries on your behalf. It generates SQL; you run it however you like.

---

## How it works

```
schema.asl  ──► axel generate ──► migration.sql ──► axel up ──► PostgreSQL
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

Run `axel generate -n "initial schema"` to diff against the last migration and produce a `.sql` file. Run `axel up` to apply it.

Full reference: [docs/asl.md](docs/asl.md)

---

## Query language (AQL)

Write queries in `.aql` files and compile them to parameterized SQL:

```aql
select User {
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
# axel.yaml
database-url: postgres://user:pass@localhost:5432/mydb
schema-path: ./schema.asl
migrations-dir: ./migrations
```

```sh
axel -c axel.yaml <command>
```

### Commands

| Command         | Description                                |
| --------------- | ------------------------------------------ |
| `axel validate` | Parse and validate a `.asl` schema file    |
| `axel compile`  | Compile an AQL query to parameterized SQL  |
| `axel generate` | Diff schema and write a new migration file |
| `axel up`       | Apply all pending migrations               |
| `axel down <n>` | Roll back the last N migrations            |
| `axel status`   | Show migration state                       |

```sh
# Validate schema
axel -d . validate

# Compile a query (use single quotes to prevent shell $expansion)
axel -d . compile --aql 'select User { id, email } filter .id = $id;'
axel -d . compile --file queries/get_user.aql --out queries/get_user.sql

# Migrations
axel -d . generate -n "add posts table"
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
axel -d . generate -n "initial schema"

# 4. Apply it
axel -d . up

# 5. Write queries
vim queries/get_users.aql

# 6. Compile to SQL
axel -d . compile --file queries/get_users.aql

# 7. Execute the SQL in your application however you like
```

---

## Documentation

- [docs/asl.md](docs/asl.md) — Axel Schema Language reference
- [docs/aql.md](docs/aql.md) — Axel Query Language reference
- [docs/cli.md](docs/cli.md) — CLI commands and flags

---

## License

See [LICENSE](./LICENSE) for details.
