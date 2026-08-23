# Axel

This project uses [Axel](https://github.com/struckchure/axel) for its PostgreSQL schema and
queries: `.asl` files compile to migration SQL, `.aql` files compile to parameterized query
strings. Axel is a compiler, not an ORM — it never executes a query for you.

Before touching any `.asl` or `.aql` file, read:

- **[`SKILL.md`](./SKILL.md)** — the mental model, the edit→validate→diff→up loop, and the mistakes
  that actually happen. Read this first; it is short.
- [`references/asl.md`](./references/asl.md) — full schema language
- [`references/aql.md`](./references/aql.md) — full query grammar
- [`references/codegen.md`](./references/codegen.md) — client code generation for Go and TypeScript, transactions, and plugins
- [`references/cli.md`](./references/cli.md) — every command, `axel.yaml`, codegen

The four commands worth running instead of guessing, none of which need a database except the last:

```sh
axel fmt -w .                                  # format .asl / .aql files in place (always run after editing)
axel validate                                  # does the schema resolve?
axel compile --aql 'select User { id }'        # what SQL does this shape produce?
axel diff -n "wip" && cat migrations/*/up.sql  # what DDL does this change produce?
```
