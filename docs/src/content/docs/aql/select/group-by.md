---
title: "Group By & Having — AQL"
description: "Multi-row aggregation, grouped projections, and having clauses in AQL"
---

# Group By & Having

AQL supports grouped aggregation queries using `group by` and `having` clauses on `select` and `multi select` statements.

## Grouped select

To group records and compute aggregates per group, specify a `group by` clause and select the grouping properties and aggregate fields in the shape:

```aql
multi select Transaction {
  status,
  order_count := count(),
  total_volume := sum(.amount)<int64>
}
group by .status;
```

```sql
SELECT
  t.status AS status,
  COUNT(*) AS order_count,
  (SUM(t.amount))::BIGINT AS total_volume
FROM "transaction" t
GROUP BY t.status;
```

## Filter (WHERE) and Having (HAVING)

- `filter` filters individual rows **before** grouping (compiles to SQL `WHERE`).
- `having` filters groups **after** aggregation (compiles to SQL `HAVING`).

```aql
multi select Transaction {
  status,
  total_volume := sum(.amount)<int64>,
  successful_volume := sum(.amount)<int64> filter .status = TransactionStatus.Successful,
  order_count := count()
}
filter .created_at >= $since
group by .status
having count() >= $min_orders and sum(.amount) > $min_volume
order by total_volume desc
limit $limit;
```

```sql
-- $1: since (datetime)
-- $2: min_orders (int64)
-- $3: min_volume (int64)
-- $4: limit (int32)
SELECT
  t.status AS status,
  (SUM(t.amount))::BIGINT AS total_volume,
  (SUM(t.amount) FILTER (WHERE t.status = 'Successful'))::BIGINT AS successful_volume,
  COUNT(*) AS order_count
FROM "transaction" t
WHERE t.created_at >= $1
GROUP BY t.status
HAVING COUNT(*) >= $2 AND SUM(t.amount) > $3
ORDER BY total_volume DESC
LIMIT $4;
```

## Multiple grouping columns

You can group by multiple fields by separating them with commas:

```aql
multi select Transaction {
  status,
  type,
  total := sum(.amount)<int64>,
  count := count()
}
group by .status, .type;
```

```sql
SELECT
  t.status AS status,
  t.type AS type,
  (SUM(t.amount))::BIGINT AS total,
  COUNT(*) AS count
FROM "transaction" t
GROUP BY t.status, t.type;
```

## Rules

- **Shape requirements:** In a grouped select, every shape field must either be a grouping column, an aggregate expression (`count()`, `sum()`, `avg()`, `min()`, `max()`), or a computed expression over group columns and aggregates. Ungrouped non-aggregate columns are rejected with a compile error.
- **No wildcard:** `*` splat is not permitted in a grouped query.
- **Conditional aggregates:** Per-field `filter` (`FILTER (WHERE ...)`) is supported on aggregate fields in grouped queries.
- **Single vs multi select:** `multi select` returns all groups (with optional `limit` and `offset`); `select` returns a single group (with implicit `LIMIT 1`).
