---
title: "Query Language (AQL)"
description: "Write expressive queries that compile to parameterized PostgreSQL SQL"
---

# Axel Query Language (AQL)

AQL is a query language that compiles to parameterized PostgreSQL SQL. You write queries in `.aql` files; Axel outputs SQL strings you execute with your language's standard driver. Axel never runs queries for you.

:::tip[AOT Queries over Runtime Builders]
Axel strongly encourages **Ahead-of-Time (AOT) query compilation**. Instead of constructing queries at runtime via dynamic query builder APIs, you write `.aql` files and compile them at build time with `axel codegen`. This validates query syntax against your schema, eliminates runtime query-building overhead, and generates exact, type-safe parameters and response models for your target language (Go, TypeScript).
:::

```
query.aql
```

## How AQL is organized

The reference is split by feature:

- **[Parameters](/aql/parameters)** — named, optional, and typed query parameters.
- **[Select](/aql/select)** — single vs multi, shapes, filters, ordering, nested links, and aggregates.
- **[Insert](/aql/insert)** — inserting rows, links, and `unless conflict` upserts.
- **[Update](/aql/update)** — updates and partial updates.
- **[Delete](/aql/delete)** — deleting rows.
- **[Expressions](/aql/expressions)** — operators, literals, path expressions, and casts.
- **[With](/aql/with)** — bind a subquery once with `with (...)` and reuse it across the query.
- **[Directives](/aql/directives)** — `@name` / `@request` / `@response` codegen metadata.
- **[Grammar reference](/aql/grammar)** — the full AQL grammar.

## Output format

Every compiled query produces:

- A positional-parameter SQL string (`$1`, `$2`, ...)
- A comment header mapping parameter names to positions

```sql
-- $1: email (str)
-- $2: active (bool)
SELECT u.id AS id, u.email AS email
FROM "user" u
WHERE u.email = $1 AND u.active = $2;
```

The parameter order matches first-appearance order in the query.
