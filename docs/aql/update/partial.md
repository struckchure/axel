---
title: Partial updates — AQL
description: Optional params in set, and keeping current values
---

# Partial updates

An optional parameter (`$name?`) in a `set` clause is plain nullable: when the value is `null`, the
column is **written to `NULL`**. (This differs from an optional parameter in a *filter*, where `null`
skips the condition — see [Optional parameters](/aql/parameters/optional).)

To leave a column **unchanged** when a value is absent, coalesce the parameter to the column's
current value with `?? .field`:

```aql
update Application
filter .id = $id
set {
  status              := $status?,                        -- null → sets the column to NULL
  build_system        := $build_system? ?? .build_system  -- null → keeps the current value
};
```

```sql
-- $1: status
-- $2: build_system
-- $3: id
UPDATE "application" a SET
  status = $1,
  build_system = COALESCE($2::TEXT, a.build_system)
WHERE a.id = $3
RETURNING *;
```

The `??` cast (`$2::TEXT` here) is the column's SQL type, so the parameter's type is determinable
even when its value is `null`.
