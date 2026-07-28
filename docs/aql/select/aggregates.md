---
title: Aggregates — AQL
description: count and aggregate selects
---

# Aggregate select

## count

```aql
select count(User);
```

```sql
SELECT COUNT(*) FROM (
  SELECT 1 FROM "user" u
) _agg;
```

With a filter:

```aql
select count(User filter .active = true);
```

```sql
SELECT COUNT(*) FROM (
  SELECT 1 FROM "user" u
  WHERE u.active = true
) _agg;
```

## As a scalar subquery

An aggregate can also be wrapped in parentheses and used as a **scalar operand**
anywhere an expression is accepted — inside a `filter`, or on the right-hand
side of an `update` `set` assignment. It compiles to the same `SELECT COUNT(*)`
wrapped in parentheses, and its `filter` may correlate to the outer row.

```aql
multi select User { id, email }
filter (select count(Post filter .author.id = User.id)) > 0;
```

```sql
SELECT
  u.id AS id,
  u.email AS email
FROM "user" u
WHERE (SELECT COUNT(*) FROM (
  SELECT 1 FROM "post" p
  WHERE p.author = u.id
) _agg) > 0;
```

The inner `filter .author.id = User.id` references the outer alias (`u.id`), so
the count is evaluated per user. The same form works as an assignment value —
`set { has_posts := (select count(Post filter .author.id = User.id)) > 0 }`.

> **Note:** an aggregate subquery is only valid as an expression operand. It is
> not accepted as a [computed shape field](/aql/select/computed) value
> (`{ n := (select count(...)) }`).
