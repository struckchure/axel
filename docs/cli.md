# Axel CLI

Axel is invoked as `axel <command>`. Commands split into two groups:

- **Schema commands** — compile ASL and manage PostgreSQL migrations (require a DB connection)
- **Query commands** — parse and compile AQL to SQL (no DB connection)

---

## Global flags

These flags are accepted by all commands.

| Flag              | Short | Description                                                    |
|-------------------|-------|----------------------------------------------------------------|
| `--dir`           | `-d`  | Project directory — auto-discovers `axel.yaml`, `schema.asl`, or `default.asl` |
| `--config`        | `-c`  | Explicit config file path (overrides `--dir`)                  |
| `--url`           | `-u`  | PostgreSQL connection URL                                      |
| `--schema-path`   |       | Explicit schema file path (overrides `--dir`)                  |
| `--migrations-dir`|       | Migrations directory (overrides `--dir`)                       |

### Project directory (`--dir`)

The simplest way to configure axel. Point it at your project folder and it figures out the rest:

```sh
axel -d ./myproject validate
axel -d ./myproject compile --aql 'select User { id, email };'
axel -d ./myproject up
```

Discovery order inside `--dir`:

1. `axel.yaml` — if found, loaded as the full config
2. `schema.asl` — used as the schema if no `axel.yaml`
3. `default.asl` — fallback schema name

### Config file (`--config`)

For explicit control, or when the config lives outside the project directory:

```yaml
database-url: postgres://user:pass@localhost:5432/mydb
schema-path: ./schema/main.asl
migrations-dir: ./migrations
```

```sh
axel --config axel.yaml <command>
```

---

## Schema commands

### `axel generate`

Diffs the current `.asl` schema against the last migration and writes a new `.sql` migration file.

```sh
axel generate --name "add users table"
axel generate -n "add users table"
```

| Flag     | Short | Description                          |
|----------|-------|--------------------------------------|
| `--name` | `-n`  | Human-readable label for the migration |

The generated file is written to `--migrations-dir` with a timestamp prefix, e.g.:

```
axel/migrations/20240418120000_add_users_table.sql
```

---

### `axel up`

Applies all pending migrations in order.

```sh
axel up
axel --url postgres://... up
```

Axel tracks applied migrations in a `_axel_migrations` table it creates on first run.

---

### `axel down`

Rolls back the last N migrations.

```sh
axel down 1    # roll back the most recent migration
axel down 3    # roll back the last 3 migrations
```

```
axel down <steps>
```

---

### `axel status`

Prints the state of all known migrations.

```sh
axel status
```

Example output:

```
20240101000000_create_users    applied
20240102000000_add_posts       applied
20240418120000_add_comments    pending
```

---

## Query commands

These commands compile AQL queries to SQL. They read the schema file but do not connect to a database.

### `axel compile`

Compiles an AQL query to parameterized SQL and prints it to stdout.

```sh
# Inline query — use single quotes so the shell doesn't expand $params
axel compile --aql 'select User { id, email } filter .id = $id;'

# From a file (recommended for queries with parameters)
axel compile --file queries/get_users.aql

# Write output to a file
axel compile --file queries/get_users.aql --out queries/get_users.sql
```

> **Shell quoting:** Always use single quotes around `--aql` values. Double quotes cause the shell to expand `$param` as a shell variable before axel sees it.

| Flag            | Short | Default           | Description                              |
|-----------------|-------|-------------------|------------------------------------------|
| `--aql`         |       |                   | AQL query string (mutually exclusive with `--file`) |
| `--file`        | `-f`  |                   | Path to a `.aql` file                    |
| `--out`         | `-o`  | stdout            | Write compiled SQL to this file          |
| `--schema-path` |       | `axel/schema.asl` | Schema to compile against                |

Example output:

```sql
-- $1: active (bool)
-- $2: min_age (int32)
SELECT
  u.id AS id,
  u.email AS email
FROM "user" u
WHERE u.active = $1 AND u.age >= $2;
```

---

### `axel validate`

Parses and validates an ASL schema file. Exits with a non-zero status and prints errors if the schema is invalid.

```sh
axel validate
axel validate --schema axel/schema.asl
```

| Flag       | Short | Default           | Description              |
|------------|-------|-------------------|--------------------------|
| `--schema` | `-s`  | `axel/schema.asl` | Path to the `.asl` file  |

On success:

```
schema "axel/schema.asl" is valid (5 types)
```

On failure (exits 1):

```
schema validation failed:
  • type "Post" extends unknown type "Taggable"
  • type "Comment" has a cycle in its inheritance chain
```

---

## Typical workflow

```sh
# 1. Write your schema
vim axel/schema.asl

# 2. Validate it
axel validate

# 3. Generate a migration
axel generate -n "initial schema"

# 4. Apply the migration
axel up

# 5. Write a query
vim queries/get_posts.aql

# 6. Compile it to SQL
axel compile --file queries/get_posts.aql

# 7. Run the SQL in your application however you like
```

---

## Environment variable

`DATABASE_URL` is read as the default connection URL when `--url` and `--config` are not provided.

```sh
export DATABASE_URL=postgres://user:pass@localhost:5432/mydb
axel up
```
