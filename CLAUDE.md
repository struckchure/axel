# Axel

Axel is an **ahead-of-time compiler for PostgreSQL**, written in Go. It is not an ORM and has no
runtime: `.asl` (Axel Schema Language) files compile to migration SQL, `.aql` (Axel Query Language)
files compile to parameterized SQL strings plus typed client code. Nothing happens at query time
that is not visible in the generated SQL.

That framing settles most design questions. If a change would make Axel *execute* something on the
user's behalf at query time, it probably belongs in the generated client, not in the compiler.

## Workflow

- NEVER run browser tests (opening the app in a browser, taking screenshots, driving the UI) without
  asking first. Verify with builds, `curl`, and unit tests instead, and only open a browser when the
  user explicitly agrees.
- Git commit messages: do NOT append the `Co-Authored-By: Claude ...` line or any Claude / Claude
  Code attribution footer.

## Layout

| Path | What lives there |
|---|---|
| `cmd/` | The `axel` CLI (cobra): `validate`, `diff`, `up`, `down`, `status`, `compile`, `codegen`, `fmt`, `run`, `repl`, `studio`, `lsp`, `init` |
| `core/asl/` | ASL parser (participle), formatter, resolver → `SchemaIR` |
| `core/aql/` | AQL parser, formatter, printer |
| `core/compiler/` | AQL → SQL lowering, params, functions, `with` blocks, inlining |
| `core/codegen/` | Generator-agnostic descriptors (`SchemaDescriptor`, `QueryDescriptor`) + plugin protocol |
| `core/` (root) | Schema diff, migration generation and execution, DDL SQL generator |
| `core/lsp/` | Language server used by the editor extensions |
| `generators/golang/`, `generators/typescript/` | Built-in client generators |
| `studio/` | templ + Tailwind web UI (`axel studio`) |
| `tests/` | The integration suite — most behavioral coverage lives here, not next to the code |
| `tools/agent/` | The shipped agent guide (SKILL.md + references). Keep in sync with behavior changes |
| `tools/vscode/`, `tools/zed/` | Editor extensions, tree-sitter grammars, TextMate syntaxes |
| `docs/` | Astro + Starlight documentation site (TypeScript/Bun — see below) |

## Building and testing

```sh
go build ./...                       # build everything
go build -o /tmp/axel ./cmd          # build the CLI to try a change end to end
go test ./tests/ -run Something      # the integration suite (no database needed)
go test ./...                        # everything
```

The test suite needs no database: it asserts on generated SQL strings and generated client source.
Helpers already exist in `tests/` — `compileAQL`, `parseSchema`, `genUp`, `genMigration`,
`buildQueryDesc`, `readFile`. Reuse them rather than re-deriving setup.

Behavior changes belong in `tests/`, one file per concern, with the *reason* the behavior exists
written as a comment above the test — that convention is consistent across the suite and worth
matching.

## Verifying compiler behavior

The compiler is the source of truth and it is fast. Prefer running it over reasoning about it:

```sh
axel fmt -w .                                   # canonical formatting (run after EVERY .asl/.aql edit)
axel validate                                   # parse + resolve; no database needed
axel compile --schema-path schema.asl --aql 'select User { id }'
axel diff -n "wip" && cat migrations/*/up.sql   # what DDL does this change produce?
```

Single-quote inline queries or the shell eats `$params`. Never run `axel compile -d .` without
`--output-dir` — it writes a `.sql` file next to every `.aql` in the tree.

## Naming rules that keep biting

These are decided in `core/asl/resolver.go` and must agree across the DDL generator, the AQL
compiler, the generated clients, and studio:

- A single `link author: User` produces an FK column named **`author`** — *not* `author_id`.
- A `multi link tags: Tag` on `Post` produces junction table **`post_tags`** (`{owner}_{field}`).
- Junction FK columns are named after the tables they reference (`post`, `tag`), except when a multi
  link points at its own type — then the target side falls back to the field name. Derive them from
  `asl.JunctionColumns`, never by hand.
- A `multi` **scalar** is an array column, not a junction table.

## Agent-facing docs

`tools/agent/` ships to users via `npx skills add struckchure/axel`, so it is a product surface, not
internal notes. When compiler or codegen behavior changes, update `tools/agent/SKILL.md` and the
matching `tools/agent/references/*.md`, and the corresponding page under
`docs/src/content/docs/`. Every example in those files should be one that was actually compiled.

## The docs site (`docs/`) — Bun, not Node

`docs/` is an Astro + Starlight site. Default to Bun there.

- `bun install`, `bun run dev`, `bun run build` (or `bun --cwd docs dev` from the repo root)
- Use `bun <file>` instead of `node <file>` / `ts-node <file>`; `bunx <pkg>` instead of `npx <pkg>`
- Bun loads `.env` automatically — do not add `dotenv`
- New pages need a sidebar entry in `docs/astro.config.mjs`

`tools/vscode/` is likewise a Bun/TypeScript package.
