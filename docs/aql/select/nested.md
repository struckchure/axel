---
title: Nested shapes (links) — AQL
description: Selecting linked types as JSON, with no N+1
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
