---
title: "Nested shapes (links) — AQL"
description: "Selecting linked types as JSON, with no N+1. Configurable between correlated subquery (json_agg) and LEFT JOIN LATERAL strategies."
---

# Nested shapes (links)

Shapes can include linked types. Axel compiles nested shapes into a single SQL query using `row_to_json` or `json_agg` — no N+1.

## Single link

Returns a JSON object.

```aql
select Post {
  id,
  title,
  author: {
    id,
    email
  }
};
```

```sql
SELECT
  p.id AS id,
  p.title AS title,
  (SELECT row_to_json(u_author_sub)
   FROM (
     SELECT u_author.id AS id, u_author.email AS email
     FROM "user" u_author
     WHERE u_author.id = p.author_id
     LIMIT 1
   ) u_author_sub) AS author
FROM "post" p;
```

## Multi link

Returns a JSON array. Empty results return `[]` rather than `null`.

```aql
select Post {
  id,
  title,
  likes: {
    id,
    email
  }
};
```

```sql
SELECT
  p.id AS id,
  p.title AS title,
  (SELECT COALESCE(json_agg(row_to_json(u_likes_sub)), '[]')
   FROM (
     SELECT u_likes.id AS id, u_likes.email AS email
     FROM "post_likes" jt_likes
     JOIN "user" u_likes ON u_likes.id = jt_likes.user_id
     WHERE jt_likes.post_id = p.id
   ) u_likes_sub) AS likes
FROM "post" p;
```

## Deeper nesting

A link sub-shape is a full shape: it may itself select nested links, [computed fields](/aql/select/computed),
and the `*` splat — to any depth. Each link nests another correlated JSON subquery inside its parent.

```aql
select Application {
  id,
  project: {
    id,
    organization: {
      id,
      name
    }
  }
};
```

The same paths work in a `filter`: a multi-step path resolves through the intervening links down to
the target column, so `.project.organization.id` filters against project's organization FK without an
explicit join.

```aql
multi select Application {
  *,
  project: { id, organization: { id } }
}
filter .project.organization.owner = $user<uuid>
   and .project.organization.id = $organization<uuid>?;
```

## Loading strategies

By default Axel compiles nested shapes using **correlated subqueries** in the `SELECT` projection (the `query` strategy). You can switch to **`LEFT JOIN LATERAL`** instead — either globally in `axel.yaml` or per-query with a directive.

### `query` — correlated subqueries (default)

Each nested link becomes a correlated scalar subquery inside the `SELECT` list:

- Single links → `row_to_json(...)`
- Multi links → `COALESCE(json_agg(...), '[]')`

Best for most workloads. Keeps the outer query simple and lets the planner evaluate each subquery only for the rows it needs.

### `join` — LEFT JOIN LATERAL

Each nested link becomes a `LEFT JOIN LATERAL` in the `FROM` clause:

```aql
@rel_load_strategy join

select Post {
  id,
  title,
  author: { id, email },
  likes: { id, email }
};
```

```sql
SELECT p.id, p.title, author.author, likes.likes
FROM "post" p
LEFT JOIN LATERAL (
  SELECT row_to_json(u_sub) AS author
  FROM (SELECT id, email FROM "user" WHERE id = p.author_id LIMIT 1) u_sub
) author ON true
LEFT JOIN LATERAL (
  SELECT COALESCE(json_agg(row_to_json(u_sub)), '[]') AS likes
  FROM (
    SELECT u.id, u.email FROM "post_likes" jt
    JOIN "user" u ON u.id = jt.user_id
    WHERE jt.post_id = p.id
  ) u_sub
) likes ON true;
```

Prefer `join` when the planner benefits from seeing all lateral joins together — for instance, when you filter or order by columns from nested relations, or when your PostgreSQL version handles lateral joins more efficiently for your data shape.

### Configuring the strategy

**Per-query** — use the `@rel_load_strategy` directive at the top of the `.aql` file:

```aql
@rel_load_strategy join
```

**Globally** — set it in `axel.yaml` so all queries in the project use it:

```yaml
rel-load-strategy: join
```

The per-query directive takes precedence over the global setting. See [Directives](/aql/directives) for the full list of query-level options.
