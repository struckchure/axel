# Axel for coding agents

A drop-in guide that teaches any coding agent to write ASL schemas and AQL queries, drive the
`axel` CLI, and read the errors each stage produces. It is about *using* Axel in a project, not
about working on the Axel compiler itself.

| File | What is in it |
|---|---|
| `SKILL.md` | The guide: the mental model, the edit→validate→diff→up loop, and the mistakes that actually happen |
| `references/asl.md` | Full schema language: declarations, fields, links, triggers, policies, functions |
| `references/aql.md` | Full query grammar, with the SQL each construct compiles to |
| `references/cli.md` | Every command and flag, `axel.yaml`, codegen, migration layout |
| `AGENTS.md` | A short pointer to the above, for the `AGENTS.md` convention |

Every example in these files was compiled with `axel` before being written down.

## Install

With [`npx skills`](https://github.com/vercel-labs/skills) — one command, 70+ agents (Claude Code,
Codex, Cursor, OpenCode, Copilot, …):

```sh
npx skills add struckchure/axel
```

Non-interactive, for a specific agent, installed globally rather than into the current project:

```sh
npx skills add struckchure/axel --skill axel -a claude-code -g -y
```

Point it at this directory explicitly if you would rather not rely on repo-wide discovery:

```sh
npx skills add https://github.com/struckchure/axel/tree/main/tools/agent
```

The whole directory is installed, so the `references/` files travel with the guide.

### By hand

It is plain Markdown — copy it wherever your tool reads instructions:

| Tool | Destination |
|---|---|
| Claude Code | `~/.claude/skills/axel/` (or `.claude/skills/axel/` for one project) |
| `AGENTS.md` convention | Copy the directory into the project and point your root `AGENTS.md` at `SKILL.md` |
| Cursor | `.cursor/rules/axel.mdc` |
| GitHub Copilot | `.github/copilot-instructions.md` |

```sh
git clone https://github.com/struckchure/axel.git /tmp/axel
mkdir -p ~/.claude/skills/axel && cp -r /tmp/axel/tools/agent/. ~/.claude/skills/axel/
```

## Just the docs

If a tool takes a documentation URL instead, the site publishes itself in machine-readable form:

- [llms.txt](https://struckchure.github.io/axel/llms.txt) — an index of every page
- [llms-full.txt](https://struckchure.github.io/axel/llms-full.txt) — the whole documentation, one file
