---
title: Query Language (AQL)
description: Write expressive queries that compile to parameterized PostgreSQL SQL
---

# Axel Query Language (AQL)

AQL is a query language that compiles to parameterized PostgreSQL SQL. You write queries; Axel outputs SQL strings you execute however you like. Axel never runs queries for you.

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
