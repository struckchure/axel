---
title: "CLI Reference"
description: "All Axel commands, flags, and usage examples"
---

# Axel CLI

Axel is invoked as `axel <command>`. Commands split into two groups:

- **Schema commands** — compile ASL and manage PostgreSQL migrations (require a DB connection)
- **Query commands** — parse, compile, and generate code from AQL (no DB connection required)

---

## Global flags

These flags are accepted by all commands.

| Flag               | Short | Description                                                                     |
|--------------------|-------|---------------------------------------------------------------------------------|
| `--dir`            | `-d`  | Project directory — auto-discovers `axel.yaml`, `schema.asl`, `default.asl`, or `schema/` |
| `--config`         | `-c`  | Explicit config file path (overrides `--dir`)                                   |
| `--url`            | `-u`  | PostgreSQL connection URL                                                       |
| `--schema-path`    |       | Explicit schema: a file, a directory, or a glob such as `schema/*.asl` (overrides `--dir`) |
| `--migrations-dir` |       | Migrations directory (overrides `--dir`)                                        |

### Project directory (`--dir`)

The simplest way to configure Axel. Point it at your project folder and it figures out the rest:

```sh
axel -d ./myproject validate
axel -d ./myproject compile --aql 'select User { id, email };'
axel -d ./myproject up
axel -d ./myproject codegen -g go -o ./gen
```

Discovery order inside `--dir`:

1. `axel.yaml` — if found, loaded as the full config
2. `schema.asl` — used as the schema if no `axel.yaml`
3. `default.asl` — fallback schema name
4. `schema/` — a directory of `.asl` files, [merged into one schema](/asl/splitting)

### Config file (`--config`)

For explicit control, or when the config lives outside the project directory:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/struckchure/axel/main/schema.json
database-url: postgres://user:pass@localhost:5432/mydb
schema-path: ./schema/main.asl # or a glob: ./schema/*.asl
migrations-dir: ./migrations
rel-load-strategy: query # query | join

codegen:
  generator: go # go | ts
  out-dir: ./db/generated
  queries:
    - ./queries/*.aql
  options:
    package: generated
```

```sh
axel --config axel.yaml <command>
```

---

## Project initialization

### `axel init`

Scaffolds a new Axel project by creating an `axel.yaml` config file, a starter `axel/schema.asl` schema with a `Base` type, and a `migrations` directory.

```sh
axel init
axel init --dir ./myproject --url postgres://localhost:5432/mydb
```

| Flag               | Short | Description                               |
|--------------------|-------|-------------------------------------------|
| `--dir`            | `-d`  | Target directory to initialize in         |
| `--url`            | `-u`  | Database connection URL to put in config  |
| `--schema-path`    |       | Custom schema — file, directory, or glob (default: `axel/schema.asl`) |
| `--migrations-dir` |       | Custom migrations directory (default: `migrations`) |

---

## Schema commands

### `axel diff`

Diffs the current `.asl` schema against the last migration and writes a new migration.

```sh
axel diff --name "add users table"
axel diff -n "add comments table"
```

> `axel generate` is a deprecated alias for `axel diff` and still works, printing a deprecation notice.

| Flag     | Short | Description                              |
|----------|-------|------------------------------------------|
| `--name` | `-n`  | Human-readable label for the migration   |

The generated migration is written to `--migrations-dir` with a sequential version prefix:

```
migrations/
  0001/
    up.sql
    down.sql
    metadata.json
  0002/
    ...
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

---

### `axel status`

Prints the state of all known migrations.

```sh
axel status
```

Example output:

```
0001_create_users    applied
0002_add_posts       applied
0003_add_comments    pending
```

---

## Query commands

These commands work with AQL queries. They read the schema file but do not connect to a database.

### `axel validate`

Parses and validates an ASL schema file. Exits with a non-zero status on errors.

```sh
axel validate
axel validate --schema axel/schema.asl
```

| Flag       | Short | Default           | Description             |
|------------|-------|-------------------|-------------------------|
| `--schema` | `-s`  | `axel/schema.asl` | The `.asl` schema — a file, a directory, or a glob such as `schema/*.asl` |

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

### `axel compile`

Compiles an AQL query (or all queries in a project) to parameterized SQL.

#### Single-query mode

```sh
# Inline query — use single quotes so the shell doesn't expand $params
axel compile --aql 'select User { id, email } filter .id = $id'

# From a file
axel compile --file queries/get_users.aql

# Write to a file
axel compile --file queries/get_users.aql --out queries/get_users.sql
```

> **Shell quoting:** Always use single quotes around `--aql` values. Double quotes cause the shell to expand `$param` as a shell variable before Axel sees it.

#### Batch mode

When `--dir` (`-d`) is supplied without `--aql` or `--file`, Axel finds all `*.aql` files under the project directory and compiles each one.

```sh
# Compile everything in the project, write .sql files alongside the .aql files
axel -d ./myproject compile

# Write compiled .sql files to a separate directory (created automatically)
axel -d ./myproject compile --output-dir ./sql
```

| Flag            | Short | Default           | Description                                                         |
|-----------------|-------|-------------------|---------------------------------------------------------------------|
| `--aql`         |       |                   | AQL query string (mutually exclusive with `--file`)                 |
| `--file`        | `-f`  |                   | Path to a `.aql` file                                               |
| `--out`         | `-o`  | stdout            | Output `.sql` file (single-query mode)                              |
| `--output-dir`  |       |                   | Output directory for compiled `.sql` files (batch or single mode)   |
| `--schema-path` |       | `axel/schema.asl` | Schema to compile against — file, directory, or glob                |

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

### `axel fmt`

Formats `.asl` schema and `.aql` query files in canonical style, preserving comments. Paths may be files or directories (searched recursively); with no paths, the current directory is formatted.

```sh
# Print the formatted result to stdout
axel fmt schema.asl

# Rewrite files in place
axel fmt -w .

# CI check — exits non-zero and lists files that aren't formatted
axel fmt --check .
```

| Flag      | Short | Description                                                        |
|-----------|-------|--------------------------------------------------------------------|
| `--write` | `-w`  | Write the result back to each file instead of stdout               |
| `--check` |       | List files whose formatting differs and exit non-zero if any do    |

Formatting is safe: if a reformatted file would not parse back to the same structure, the original is left untouched. Invalid source is reported as an error.

---

### `axel codegen`

Generates code from your schema and compiled AQL queries. See the [Code Generation](./codegen) guide for full details.

```sh
# Go
axel -d ./myproject codegen -g go -o ./gen

# TypeScript
axel -d ./myproject codegen -g ts -o ./gen

# External generator binary
axel -d ./myproject codegen --plugin ./my-generator -o ./gen
```

| Flag            | Short | Default | Description                                              |
|-----------------|-------|---------|----------------------------------------------------------|
| `--generator`   | `-g`  |         | Built-in generator (`go` or `ts`)                        |
| `--plugin`      | `-p`  |         | Path to external generator binary                        |
| `--out-dir`     | `-o`  | `.`     | Directory to write generated files into                  |
| `--query`       | `-q`  |         | AQL file or glob pattern — repeatable                    |
| `--schema-path` |       |         | Schema file, directory or glob (default: from config or `axel/schema.asl`) |
| `--option`      |       |         | `key=value` forwarded to the generator — repeatable      |

---

## Typical workflow

```sh
# 1. Write your schema
vim schema.asl

# 2. Validate it
axel validate

# 3. Generate and apply a migration
axel diff -n "initial schema"
axel up

# 4. Write queries
vim queries/list_posts.aql

# 5. Generate typed code
axel codegen -g go -o ./gen

# 6. Compile queries to SQL (optional — codegen does this internally)
axel compile --output-dir ./sql

# 7. Use the generated code in your application
```

---

## Environment variable

`DATABASE_URL` is read as the default connection URL when `--url` and `--config` are not provided.

```sh
export DATABASE_URL=postgres://user:pass@localhost:5432/mydb
axel up
```
