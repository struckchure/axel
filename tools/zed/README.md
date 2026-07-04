# Axel — Zed extension

Syntax highlighting, comment toggling, bracket matching, and outlines for the two
Axel languages:

- **Axel Schema** — `.asl` (types, enums, links, constraints, indexes)
- **Axel Query** — `.aql` (`select` / `insert` / `update` / `delete`)

Both are backed by small Tree-sitter grammars that mirror the parsers in
`core/asl` and `core/aql`.

## Layout

```
tools/zed/
  extension.toml                     # extension manifest + grammar references
  languages/
    asl/  { config.toml, highlights.scm, brackets.scm, indents.scm, outline.scm }
    aql/  { config.toml, highlights.scm, brackets.scm, indents.scm, outline.scm }
  grammars/
    asl/  { grammar.js, package.json, tree-sitter.json, src/ }   # generated parser committed
    aql/  { grammar.js, package.json, tree-sitter.json, src/ }
```

## The `rev` requirement

Zed fetches grammars from git at a specific revision, so `[grammars.*].rev` in
`extension.toml` must be a **real commit that contains `tools/zed/grammars`**. After
committing this directory, set both `rev` values to that commit SHA.

### Local development (no push required)

Point the grammar `repository` at the local repo via a `file://` URL and use your
local commit SHA:

```toml
[grammars.asl]
repository = "file:///Users/mohammed/projects/axel"
path = "tools/zed/grammars/asl"
rev = "<local commit sha>"

[grammars.aql]
repository = "file:///Users/mohammed/projects/axel"
path = "tools/zed/grammars/aql"
rev = "<local commit sha>"
```

Get the SHA with `git rev-parse HEAD` after committing `tools/zed`.

### Distribution

Use the GitHub URL (the committed default) with the pushed commit SHA.

## Install as a dev extension

1. Commit `tools/zed` and set the two `rev` values (see above).
2. In Zed: run **`zed: install dev extension`** (command palette) and select the
   `tools/zed` directory.
3. Open `examples/basic/default.asl` and any `examples/basic/*.aql` file.

## Regenerating the parsers

The generated `src/` is committed so consumers need no toolchain. To regenerate
after editing a `grammar.js`:

```sh
cd tools/zed/grammars/asl && bunx tree-sitter-cli generate
cd tools/zed/grammars/aql && bunx tree-sitter-cli generate
```

Run the grammar corpus tests with `bunx tree-sitter-cli test` in either grammar dir.
