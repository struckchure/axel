---
title: Examples — Axel
description: Practical recipes for common things you can build with Axel
---

# Examples

Short, copy-pasteable recipes for things you'll reach for often. Each one is a
small slice of ASL and/or AQL — every snippet here compiles as-is.

| Recipe | What it shows |
| ------ | ------------- |
| [Audit timestamps & UUID keys](/examples/timestamps) | A reusable `Base` type: UUID primary keys and auto-touched `created_at` / `updated_at`. |
| [Soft deletes](/examples/soft-delete) | Hide "deleted" rows with a policy instead of removing them. |
| [Slugs from titles](/examples/slugs) | Derive a URL slug on insert/update with a function + rewrite. |
| [Multi-tenant row ownership](/examples/multi-tenancy) | Scope every row to the current user with a global + RLS policy. |
| [Expiring rows + cleanup](/examples/expiring-rows) | A TTL policy that hides stale rows, swept by a scheduled job. |
| [Upserts](/examples/upsert) | Insert-or-update in one statement with `unless conflict`. |
| [Nested data in one query](/examples/nested-data) | Fetch an object and its related rows as JSON — no N+1. |

Most recipes build on a shared `Base` type (see
[Audit timestamps & UUID keys](/examples/timestamps)); the examples show only the
fields relevant to each recipe.

New to Axel? Start with the [Tutorial](/tutorial), then come back here for
task-focused patterns.
