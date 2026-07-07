# Axel — VS Code extension

Syntax highlighting, comment toggling, bracket matching, **and a language server**
(diagnostics, hover, go-to-definition, completion) for the two Axel languages:

- **Axel Schema** — `.asl` (types, enums, links, constraints, indexes)
- **Axel Query** — `.aql` (`select` / `insert` / `update` / `delete`, `$name<type>`
  parameters, `@name` / `@request` / `@response` directives, `{ *, … }` splat)

Highlighting is provided by TextMate grammars (VS Code does not use tree-sitter
natively), mirroring the scopes chosen by the Zed extension in `tools/zed`.

## Language server

The extension launches the `axel` CLI as `axel lsp`. **You must have the `axel`
binary installed and on your `PATH`** — install it from
<https://github.com/struckchure/axel>. If `axel` isn't found, VS Code shows a
notification and only the (TextMate) highlighting is active.

- Setting **`axel.path`** — absolute path to the `axel` binary (defaults to
  looking it up on `PATH`).
- Command **Axel: Restart Language Server** — restarts the server after
  installing/updating `axel`.

## Layout

```
tools/vscode/
  package.json                       # manifest (languages + grammars + client)
  src/extension.ts                   # language client that spawns `axel lsp`
  language-configuration/
    asl.json  aql.json               # comments, brackets, auto-closing pairs
  syntaxes/
    asl.tmLanguage.json              # TextMate grammar (scope source.asl)
    aql.tmLanguage.json              # TextMate grammar (scope source.aql)
```

## Run it locally

```sh
cd tools/vscode
bun install
bun run compile
```

Then open `tools/vscode` in VS Code and press **F5** to launch an Extension
Development Host, and open any `.asl` / `.aql` file (e.g. under `examples/basic/`).

## Package a `.vsix`

```sh
cd tools/vscode
bun install
bun run package     # bun run compile + vsce package --no-dependencies
```

Install the resulting `.vsix` with the `code` CLI:

```sh
code --install-extension axel-*.vsix
```

(Or use **Extensions: Install from VSIX…** in the command palette.) Reload the
window, then run **Axel: Restart Language Server** after installing/updating `axel`.
