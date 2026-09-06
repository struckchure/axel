---
title: "Bulk Insert — AQL"
description: "Bulk insert operations using for-loops and multi-value parameters in AQL"
---

# Bulk Insert

AQL supports bulk insertions using EdgeQL-style `for ... in ...` iteration statements combined with multi-valued parameters or set literals.

```aql
var multi $conditions: str? := {'Hot', 'Cold', 'Fragile', 'Frozen'}

for $condition in $conditions {
  insert PackageCondition {
    name := $condition,
    added_by := (select User filter .email = 'alice@example.com')
  } unless conflict;
}
```

```sql
-- $1: conditions (str[])
WITH __for_iter AS (
  SELECT unnest(COALESCE($1::TEXT[], ARRAY['Hot', 'Cold', 'Fragile', 'Frozen']::TEXT[])) AS "condition"
)
INSERT INTO "package_condition" ("name", "added_by")
SELECT
  __for_iter."condition",
  (SELECT u.id FROM "user" u WHERE u.email = 'alice@example.com' LIMIT 1)
FROM __for_iter
ON CONFLICT DO NOTHING
RETURNING "id", "name", "added_by";
```

---

## How It Works

1. **`var multi $param: type? := default`**:
   - `multi` specifies that the parameter expects an array of elements (e.g. `TEXT[]`, `UUID[]`, `INT[]`).
   - `: type` annotates the element scalar type.
   - `:=` assigns an optional default array expression (e.g. a set literal `{'A', 'B'}`).

2. **`for $item in $collection { ... }`**:
   - Iterates through each element in `$collection` (which can be a parameter or an inline set literal).
   - In the loop body, `$item` can be referenced in field assignments or subqueries.
   - The loop body compiles to a PostgreSQL Common Table Expression (CTE) using `unnest(...)`, followed by `INSERT ... SELECT ... FROM __for_iter`.

---

## Examples

### Bulk Inserting with Set Literals

You can iterate over inline set literals directly:

```aql
for $role in {'Admin', 'Editor', 'Viewer'} {
  insert Role {
    name := $role
  } unless conflict;
}
```

### Bulk Insert with Related Subqueries and Upserts

Each row in the loop can execute correlated lookups and handle uniqueness conflicts:

```aql
var multi $tags: str?

for $tag in $tags {
  insert Tag {
    name := $tag,
    created_by := (select User filter .id = $user_id<uuid>)
  } unless conflict on .name else (
    update Tag set {
      usage_count := .usage_count + 1
    }
  );
}
```

### Upserting Iterator Values (`EXCLUDED`)

Inside `ON CONFLICT ... DO UPDATE`, Postgres has only two rows in scope: the existing row and
`EXCLUDED`, the row the insert proposed. The `__for_iter` CTE is *not* in scope there. When the
`else` update reuses an expression the insert already writes, Axel rewrites it to that column's
`EXCLUDED` reference:

```aql
var multi $plans: str? := {'A1|0|NGN|512|256'}

for $plan in $plans {
  insert Plan {
    name := split_part($plan, '|', 1),
    price := split_part($plan, '|', 2)<int64>,
    currency := split_part($plan, '|', 3),
    memory := split_part($plan, '|', 4)<int32>,
    cpu := split_part($plan, '|', 5)<int32>
  } unless conflict on .name else (update Plan set {
    price := split_part($plan, '|', 2)<int64>,
    memory := split_part($plan, '|', 4)<int32>
  });
}
```

```sql
-- $1: plans (str[])
WITH __for_iter AS (
  SELECT unnest(COALESCE($1::TEXT[], ARRAY['A1|0|NGN|512|256']::TEXT[])) AS "plan"
)
INSERT INTO "plan" ("name", "price", "currency", "memory", "cpu")
SELECT
  split_part(__for_iter."plan", '|', 1),
  (split_part(__for_iter."plan", '|', 2))::BIGINT,
  split_part(__for_iter."plan", '|', 3),
  (split_part(__for_iter."plan", '|', 4))::INTEGER,
  (split_part(__for_iter."plan", '|', 5))::INTEGER
FROM __for_iter
ON CONFLICT ("name") DO UPDATE SET "price" = EXCLUDED."price", "memory" = EXCLUDED."memory"
RETURNING "cpu", "currency", "id", "memory", "name", "price";
```

Only values the insert actually writes are available this way. An `else` update that derives a new
value from the iterator — a field the insert never assigns, for example — is rejected at compile
time instead of failing at run time with `missing FROM-clause entry for table "__for_iter"`.

:::note
`split_part` field positions are 1-based in Postgres; position `0` raises
`field position must be greater than zero` at run time.
:::
