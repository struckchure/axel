---
title: "Neon — Integrations"
description: "Run Axel migrations against a Neon serverless Postgres database"
---

# Neon

[Neon](https://neon.tech) is serverless Postgres with branching and
scale-to-zero. It's standard Postgres over the wire, so Axel connects with a
normal connection URL and runs the usual [`axel diff` / `axel up`](/cli) flow.

## Get the connection string

In the Neon console, open **Dashboard → Connection Details** and copy the
connection string. Neon offers two endpoint styles:

- **Direct** (`ep-<id>.<region>.aws.neon.tech`) — **use this for Axel migrations.**
- **Pooled** (`ep-<id>-pooler.<region>.aws.neon.tech`) — PgBouncer in transaction
  mode, for high-concurrency app runtime. Not for migrations.

Neon **requires TLS**, so the URL includes `sslmode=require`:

```sh
export DATABASE_URL='postgresql://<user>:<password>@ep-<id>.<region>.aws.neon.tech/<db>?sslmode=require'
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

## Branch-per-environment

Neon's database branches pair well with Axel's file-based migrations: create a
branch, point `DATABASE_URL` at its direct endpoint, and run `axel up` to bring
that branch's schema up to date. Because migrations are tracked in
`_axel_migrations`, each branch converges to the same schema independently.

```sh
# preview branch
export DATABASE_URL='postgresql://<user>:<password>@ep-preview-....aws.neon.tech/<db>?sslmode=require'
axel up
```

:::tip[Migrate against the direct endpoint]
Run `axel up` against the **non-pooled** endpoint. The `-pooler` endpoint is
transaction-mode PgBouncer and doesn't preserve session state between statements,
which migrations rely on.
:::

:::caution[Scale-to-zero cold starts]
An idle Neon database suspends; the first migration statement may pause briefly
while it wakes. This is expected — the command proceeds once the compute resumes.
:::
