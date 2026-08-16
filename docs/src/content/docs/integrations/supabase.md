---
title: "Supabase — Integrations"
description: "Run Axel migrations against a Supabase Postgres database"
---

# Supabase

[Supabase](https://supabase.com) is managed Postgres with auth, storage, and an
auto-generated API layered on top. Axel manages the **database schema** — types,
migrations, and row-level security — and Supabase serves it. There's no special
adapter: Axel connects with a normal Postgres URL.

## Get the connection string

In the Supabase dashboard, open **Project Settings → Database → Connection
string** and copy the `URI`. You'll see a few variants:

- **Direct connection** (`db.<ref>.supabase.co:5432`) — a plain session
  connection. **Use this for Axel migrations.**
- **Session pooler** (port `5432`, `...pooler.supabase.com`) — a good direct
  substitute on IPv4-only networks; also fine for migrations.
- **Transaction pooler** (port `6543`) — for high-concurrency app runtime, **not**
  for migrations (no session state across statements).

The URL already carries the database password; keep it in an env var.

```sh
export DATABASE_URL='postgresql://postgres:<password>@db.<ref>.supabase.co:5432/postgres?sslmode=require'
```

```yaml
# axel.yaml
schema-path: ./schema.asl
migrations-dir: ./migrations
database-url: $env.DATABASE_URL
```

## Apply your schema

```sh
axel validate      # parse + type-check, no DB needed
axel diff -n init  # write a migration from the schema diff
axel up            # apply pending migrations
```

`axel up` tracks applied migrations in an `_axel_migrations` table, so re-running
is safe.

## Row-level security fits naturally

Supabase gates its client APIs on Postgres **RLS**, and Axel
[policies](/asl/policies) lower straight to `CREATE POLICY` +
`ENABLE ROW LEVEL SECURITY`. Supabase connects clients through restricted roles
(`anon`, `authenticated`) while the `service_role` bypasses RLS — the same
owner-bypass model Axel documents.

A tenant-scoped example keyed on the current user. Axel reads the current user
through a [`global`](/asl/globals), which lowers to a Postgres session setting
(`current_setting('app.current_user', …)`):

```asl
global current_user: uuid;

type Document {
  required owner: uuid;
  required title: str;

  policy owner_rw for all to authenticated
    using ( .owner = global current_user )
    with check ( .owner = global current_user );
}
```

Populate that session variable per request from the authenticated user id
(Supabase exposes it as `auth.uid()`), e.g. `SET app.current_user = '<uuid>'` at
the start of the transaction. See [multi-tenant ownership](/examples/multi-tenancy)
for the full pattern, and the [append-only log](/examples/append-only-log) for
locking writes with a policy.

:::caution[Migrate against a direct connection]
Point `axel up` at the **direct** or **session pooler** connection. The transaction
pooler (port `6543`) doesn't preserve session state between statements, which
migrations rely on.
:::
