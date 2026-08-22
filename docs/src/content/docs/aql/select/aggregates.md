---
title: "Aggregates — AQL"
description: "count and aggregate selects"
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

## Aggregate shape — many aggregates in one scan

A **select whose shape fields are aggregates** computes several aggregates over the
same set in a single pass. Each field is `name := <agg>(.column)` with an optional
per-field `filter`, and the select's own `filter` (after the shape, as usual) is the
shared condition applied to every field:

```aql
select Transaction {
  success_debit  := sum(.amount) filter .type = TransactionType.Debit  and .status = TransactionStatus.Successful,
  pending_debit  := sum(.amount) filter .type = TransactionType.Debit  and .status = TransactionStatus.Pending,
  success_credit := sum(.amount) filter .type = TransactionType.Credit and .status = TransactionStatus.Successful,
  pending_credit := sum(.amount) filter .type = TransactionType.Credit and .status = TransactionStatus.Pending,
}
filter (.sender_id = $api_key_id and .sender_entity = TransactionActorEntity.ApiKey)
    or (.reciever_id = $api_key_id and .reciever_entity = TransactionActorEntity.ApiKey);
```

Each field lowers to a Postgres [`FILTER (WHERE …)`](https://www.postgresql.org/docs/current/sql-expressions.html#SYNTAX-AGGREGATES)
aggregate, so the whole query is **one scan** — no correlated subqueries:

```sql
SELECT
  SUM(t.amount) FILTER (WHERE t.type = 'Debit'  AND t.status = 'Successful') AS success_debit,
  SUM(t.amount) FILTER (WHERE t.type = 'Debit'  AND t.status = 'Pending')    AS pending_debit,
  SUM(t.amount) FILTER (WHERE t.type = 'Credit' AND t.status = 'Successful') AS success_credit,
  SUM(t.amount) FILTER (WHERE t.type = 'Credit' AND t.status = 'Pending')    AS pending_credit
FROM "transaction" t
WHERE (t.sender_id = $1 AND t.sender_entity = 'ApiKey')
   OR (t.reciever_id = $1 AND t.reciever_entity = 'ApiKey');
```

The result is a **single row** (one `*Row` in generated code); `multi`, `order by`,
`limit`, and `offset` are not allowed.

### Rules

- Aggregate functions: `sum`, `avg`, `min`, `max`, `count`. `count()` (no argument)
  is `COUNT(*)`; the others take an argument expression (such as `.column`, or a math / function expression like `min(haversine(.loc.lat, .loc.lon, $target_lat, $target_lon))`). The per-field `filter` is
  optional.
- A shape is an **aggregate shape** as soon as one field is an aggregate; **every**
  field must then be an aggregate — mixing aggregates with plain row fields requires
  a **[Group By clause](/aql/select/group-by)**.
- **Result types.** Aggregate fields are nullable (an aggregate over zero rows is
  `NULL`). `count` is `int64`; `min`/`max` keep the column's type. `sum` and `avg`
  change type in Postgres (e.g. `sum` of a `bigint` column is `numeric`), so add a
  cast to pin the generated type — `sum(.amount)<int64>` — otherwise the field is
  typed as `any` and code generation warns.
