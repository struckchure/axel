---
title: Studio
description: Browse and edit your database in the browser with Axel Studio
---

# Studio

Axel Studio is a browser-based database viewer and editor in the style of Neon /
Prisma Studio. It reads your schema as ASL types, browses table data through a
typed grid, and edits rows — inserts, updates, deletes, and links — by applying
[AQL](/aql/) under the hood. It ships with the `axel` binary (assets are embedded,
so it runs from any directory).

```sh
axel studio
# → Axel Studio listening on http://localhost:4530
```

## Connection & schema

Studio needs a PostgreSQL connection and, for its schema-aware features, an `.asl`
file. Both are resolved the same way the other commands resolve them:

| Value | Resolution order |
|---|---|
| Database URL | `--url` → `axel.yaml` `database-url` → `AXEL_DATABASE_URL` / `DATABASE_URL` |
| Schema path  | `--schema-path` → `axel.yaml` `schema-path` → `AXEL_SCHEMA_PATH` → auto-discovered `default.asl` |

```sh
# Explicit
axel studio --url 'postgres://user:pass@localhost:5432/app?sslmode=disable' \
            --schema-path ./schema.asl

# Or from a project's axel.yaml
axel studio -d ./my-project
```

| Flag | Default | Description |
|---|---|---|
| `--addr` | `:4530` | Listen address |
| `--url` / `-u` | — | PostgreSQL connection URL |
| `--schema-path` | — | Path to an `.asl` schema |
| `--dir` / `-d` | — | Project directory (auto-discovers `axel.yaml`) |

### What lights up when

Studio degrades gracefully so the UI always renders:

- **Live database + schema** — the full experience: the sidebar lists your ASL
  types, data reads run as AQL, the AQL console executes, and editing is enabled.
- **Live database, no schema** — falls back to PostgreSQL introspection: tables
  come from `information_schema` and the AQL console is disabled (raw SQL still
  works).
- **No database reachable** — serves representative **sample data** so you can
  see the interface; the consoles show connect-a-database guidance.

## The workspace

A schema sidebar (with a live filter) on the left; the selected table opens with
four tabs:

- **Data** — a typed grid with sticky headers, click-to-sort columns, and
  pagination. NULLs, booleans, timestamps, JSON, and links are rendered
  distinctly; primary keys and foreign keys are badged.
- **Structure** — columns with their ASL and SQL types, nullability, defaults,
  and keys (PK / FK / multi-link).
- **AQL** — a console that runs read & write AQL against the connected database
  and shows the compiled SQL. Without a live database it compiles-only (validates
  and lowers to SQL without executing).
- **SQL** — a raw SQL console for read-only queries.

## Editing data

On a live, schema-backed table the grid is editable — changes stage locally and
apply together, so you review before anything hits the database:

- **Edit** — double-click a scalar cell and type. Edited cells are highlighted.
- **Insert** — *Insert row* adds a draft row to fill in.
- **Delete** — hover a row's index and click ×.
- **Save / Discard** — a review bar shows the pending count (e.g. *2 updates · 1
  delete*). **Save** applies everything as AQL `insert` / `update` / `delete`;
  **Discard** reloads the page.

### Links

Relationships are editable too:

- **Single links** render as a picker — a dropdown of candidate rows from the
  target type (labeled by name/title/email). Saving resolves the choice with
  `link := (select Target filter .id = $id)`.
- **Multi links** render as **tag chips** — add from a dropdown, remove with ×.
  Saving reconciles the selection against the link's junction table.

## Row-level security

Studio connects with the URL you give it, so it runs as that role. If the role is
subject to [RLS policies](/asl/policies), the data grid and AQL reads honor them —
e.g. a `hide_expired` policy keeps TTL-expired rows out of the grid. Connect with
a non-owner application role to see policies applied (the table owner bypasses
RLS). This makes Studio a faithful view of what your application actually sees.

## Development

From the repo, `task dev` runs templ, Tailwind, and the server together with hot
reload — use it instead of a bare `go run` so styles stay fresh.
