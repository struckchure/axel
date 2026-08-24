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
