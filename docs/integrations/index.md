---
title: Integrations — Overview
description: Postgres providers (Supabase, Neon) and language clients (TypeScript, Go)
---

# Integrations

Axel sits between **standard PostgreSQL** and your application. On the database
side, anything that speaks the Postgres wire protocol works — including managed
providers like [Supabase](/integrations/supabase) and [Neon](/integrations/neon).
On the application side, Axel [generates a typed client](/codegen) for
[TypeScript](/integrations/typescript) and [Go](/integrations/golang) from your
`.aql` queries. There's no provider-specific adapter: you point Axel's connection
string at the database and run the usual [`axel diff` / `axel up`](/cli) flow.

## Connecting

Axel reads the connection string from, in order of precedence:

1. the `--url` / `-u` flag,
2. the `database-url` key in `axel.yaml`,
3. the `AXEL_DATABASE_URL` env var, then `DATABASE_URL`.

Config values support `$env.NAME` references, so keep the secret out of the file:

```yaml
# axel.yaml
schema-path: ./schema.asl
migrations-dir: ./migrations
database-url: $env.DATABASE_URL
```

```sh
export DATABASE_URL='postgresql://user:pass@host:5432/dbname?sslmode=require'
axel up
```

::: tip Direct vs. pooled connections
Migrations run **DDL** (`CREATE TABLE`, `ALTER TABLE`, `CREATE POLICY`, …). Point
`axel diff` / `axel up` at a **direct / session-mode** connection, not a
transaction-mode pooler (PgBouncer in transaction mode doesn't keep session state
across statements). Both Supabase and Neon expose a direct endpoint alongside
their pooler — use it for migrations. Your application's runtime queries can still
use the pooled endpoint.
:::

::: tip SSL
Managed providers require TLS. Append `?sslmode=require` to the connection string
if the provider's copy-paste URL doesn't already include it.
:::

## Row-level security travels well

Both Supabase and Neon are ordinary Postgres, so Axel's [policies](/asl/policies)
lower to the same `CREATE POLICY` + `ENABLE ROW LEVEL SECURITY` everywhere.
Supabase in particular builds its client authorization on RLS — see the
[append-only log](/examples/append-only-log) and
[multi-tenant ownership](/examples/multi-tenancy) examples for patterns that map
directly onto it.

## Database providers

- [Supabase](/integrations/supabase)
- [Neon](/integrations/neon)

## Language clients

- [TypeScript](/integrations/typescript)
- [Go](/integrations/golang)
