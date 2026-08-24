# Axel CLI reference

Every command reads a schema; only `up`, `down`, `status` and `studio` need a database.

## Global flags

| Flag | Short | Meaning |
|---|---|---|
| `--dir` | `-d` | Project directory. Auto-discovers `axel.yaml`, then `schema.asl`, `default.asl`, `schema/` |
| `--config` | `-c` | Explicit config path (overrides `--dir`) |
| `--url` | `-u` | PostgreSQL URL (else `AXEL_DATABASE_URL`, then `DATABASE_URL`, then `.env`) |
| `--schema-path` | | Schema file, directory, or glob (`schema/*.asl`, `schema/**/*.asl`) |
| `--migrations-dir` | | Migrations directory |

## axel.yaml

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/struckchure/axel/main/schema.json
database-url: postgres://user:pass@localhost:5432/mydb   # supports $env.VAR / ${env.VAR}
schema-path: schema/*.asl        # file, directory, or glob
migrations-dir: ./migrations
rel-load-strategy: query         # query (json_agg subquery) | join (LEFT JOIN LATERAL)

codegen:
  generator: go                  # go | ts
  out-dir: ./db/generated
  queries:
    - ./queries/*.aql
  options:
    package: generated
```

## Commands

| Command | Purpose |
|---|---|
| `axel init [-d dir] [-u url]` | Scaffold `axel.yaml`, `axel/schema.asl`, `migrations/` |
| `axel validate [-s spec]` | Parse + resolve + validate. No database. Exits non-zero on error |
| `axel diff -n "label"` | Diff schema vs last migration → `migrations/NNNN/{up,down}.sql` + `metadata.json` |
| `axel up` | Apply pending migrations; tracked in the `_axel_migrations` table |
| `axel down N` | Roll back the last N migrations |
| `axel status` | List migrations as applied / pending |
| `axel compile` | Compile AQL to parameterized SQL |
| `axel codegen` | Generate a typed client from the schema + queries |
| `axel fmt` | Canonical formatting for `.asl` and `.aql` |
| `axel run` | Execute an AQL query directly against the database |
| `axel studio` | Browser database viewer/editor with AQL and SQL consoles |
| `axel lsp` | Language server over stdio (what the Zed/VS Code extensions run) |

### run

```sh
# Execute inline query with positional parameters
axel run -c "multi select User { id, name } limit \$limit;" limit=20

# Execute from file with JSON parameters
axel run -f queries/get_users.aql -p '{"limit": 20}'

# With relaxed JSON / object parameters
axel run queries/get_users.aql "params={skip: 1, limit: 20}"
```

### compile

```sh
axel compile --aql 'select User { id, email } filter .id = $id'   # single-quote it
axel compile -f queries/get_users.aql
axel compile -f queries/get_users.aql -o queries/get_users.sql
axel -d ./project compile --output-dir ./sql                      # batch: every *.aql in the project
```

### codegen

```sh
axel -d ./project codegen -g go -o ./gen
axel -d ./project codegen -g ts -o ./gen
axel -d ./project codegen --plugin ./my-generator -o ./gen
axel codegen -g go -q 'queries/**/*.aql' -o ./gen --option package=db
```

An external plugin reads a JSON `CodegenRequest` on stdin and writes a `CodegenResponse` on stdout.

### fmt

Always run `axel fmt -w .` (or `axel fmt -w <path>`) after editing or generating `.asl` and `.aql` files to apply canonical formatting in place.

```sh
axel fmt schema.asl        # to stdout
axel fmt -w .              # rewrite in place (recommended after every edit)
axel fmt --check .         # CI: lists unformatted files, exits non-zero
```

Inside a type body, members are grouped into blocks — properties and links (then computed fields),
constraints, indexes, policies, triggers — separated by a single blank line, with no blank lines
within a block. Fields keep their written order (it decides column order); the other kinds are
diffed by name, so regrouping them never affects a migration.

Formatting is safe: if the reformatted text would not parse back to the same structure, the original
is left alone.

## Migration layout

```
migrations/
  0001/
    up.sql
    down.sql
    metadata.json
```

Never hand-edit these. Change the `.asl` and re-run `axel diff`. Axel diffs *structure*, so a
renamed column reads as a drop plus an add — check `up.sql` before `axel up` and write the rename by
hand as a separate migration if that matters.
