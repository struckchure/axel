---
title: Named parameters — AQL
description: The basics of $name query parameters
---

# Named parameters

Named parameters use a `$` prefix. They are collected in order of first appearance and mapped to positional `$N` SQL parameters.

```aql
select User filter .email = $email and .active = $active;
```

```sql
-- $1: email
-- $2: active
SELECT ...
FROM "user" u
WHERE u.email = $1 AND u.active = $2;
```
