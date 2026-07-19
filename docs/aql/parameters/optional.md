---
title: Optional parameters — AQL
description: $name? and how it behaves in filters vs set clauses
---

# Optional parameters

A trailing `?` marks a parameter optional (`$email?`). In a filter, an optional parameter is **skipped when its value is null** — the condition becomes a no-op — so a single query can support present/absent filters. The generated parameter type is nullable (Go `*T`, TypeScript `field?: T | null`).

```aql
multi select User { id, email }
filter .email = $email?;
```

```sql
-- $1: email
SELECT u.id AS id, u.email AS email
FROM "user" u
WHERE ($1 IS NULL OR u.email = $1);
```

Passing `null` for `email` returns all users; passing a value filters by it.

In an `update` `set` clause an optional parameter behaves differently — `null` writes `NULL` to the
column rather than being skipped. See [Partial updates](/aql/update/partial).
