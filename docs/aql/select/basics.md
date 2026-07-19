---
title: Select basics — AQL
description: Single vs multi selects and shapes
---

# Select basics

## Single vs multi

A plain `select` returns a **single** row — Axel appends an implicit `LIMIT 1`, and code generation produces a single-row (`*Row`) result. Prefix the query with `multi` to return **all** matching rows (no implicit limit, a `[]Row` result). `limit`/`offset` are only allowed on a `multi select`.

```aql
select User { id, email };        -- one row  → LIMIT 1
multi select User { id, email };  -- all rows → no implicit limit
```

## Basic select

```aql
select User;
```

Selects all scalar properties of the type (a single row).

## Shape

A shape limits which fields are returned.

```aql
select User {
  id,
  email,
  name
};
```

```sql
SELECT u.id AS id, u.email AS email, u.name AS name
FROM "user" u
LIMIT 1;
```
