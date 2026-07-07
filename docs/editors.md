---
title: Editor setup
description: Install the Axel extensions for Zed and VS Code (highlighting + language server)
---

# Editor setup

Axel ships editor extensions for **Zed** and **VS Code**. Each provides syntax highlighting
plus a language server — live diagnostics, hover, go-to-definition, and completion — for `.asl`
schemas and `.aql` queries.

## Prerequisites

The language server *is* the `axel` CLI (`axel lsp`), so both editors need `axel` installed and
on your `PATH`. Install it first (see [Installation](/installation)) and verify:

```sh
axel version
```

If `axel` isn't found, syntax highlighting still works, but the language-server features won't
start (the extension shows a "not found" notice).

Both extensions live in the Axel repo under `tools/`, so clone it:

```sh
git clone https://github.com/struckchure/axel.git
cd axel
```

## Zed

Zed installs the extension from a local directory as a **dev extension** and compiles it on
install: the tree-sitter grammars and the Rust language-server shim. You need a
[Rust toolchain](https://rustup.rs) (Zed adds the wasm target itself).

1. Open the command palette (`cmd-shift-p`) and run **`zed: install dev extension`**.
2. Select the `tools/zed` directory in your Axel checkout.
3. Open a `.asl` or `.aql` file — highlighting and LSP features activate automatically.

To use a specific `axel` binary instead of the one on `PATH`, add to your Zed `settings.json`:

```json
{ "lsp": { "axel": { "binary": { "path": "/absolute/path/to/axel" } } } }
```

More detail: `tools/zed/README.md`.

## VS Code

Build a `.vsix` and install it with the `code` CLI. Requires [Bun](https://bun.sh).

```sh
cd tools/vscode
bun install
bun run package                     # produces axel-<version>.vsix
code --install-extension axel-*.vsix
```

Reload VS Code, then open a `.asl` or `.aql` file.

To use a specific `axel` binary, set it in **Settings** (`axel.path`):

```json
{ "axel.path": "/absolute/path/to/axel" }
```

**Developing the extension:** open the `tools/vscode` folder in VS Code and press `F5` to launch
an Extension Development Host with the extension loaded; after editing the source, re-run
`bun run package` and reinstall. Use the **Axel: Restart Language Server** command to reconnect
after installing or updating `axel`.

More detail: `tools/vscode/README.md`.

## What you get

- **Syntax highlighting** for `.asl` and `.aql`.
- **Diagnostics** — live parse/resolve errors for schemas, and parse/compile errors for queries.
- **Hover, go-to-definition, and completion**, powered by `axel lsp`.

Query files are resolved against your schema via `axel.yaml` (`schema-path`) in the workspace
root, so completion and cross-file diagnostics know your types.
