---
layout: home

hero:
  name: Axel
  text: SQL generation from schema and query languages
  tagline: Write schemas in ASL, queries in AQL. Axel compiles both to PostgreSQL SQL — migrations and parameterized query strings. No ORM, no driver, no magic.
  actions:
    - theme: brand
      text: Get Started
      link: /installation
    - theme: alt
      text: View on GitHub
      link: https://github.com/struckchure/axel

features:
  - icon: 🗂️
    title: Axel Schema Language (ASL)
    details: Define types, links, constraints, and indexes in .asl files. Axel diffs your schema and generates migration SQL automatically.
    link: /asl
    linkText: ASL reference

  - icon: 🔍
    title: Axel Query Language (AQL)
    details: Write expressive queries with shapes, filters, and nested links. Axel compiles them to parameterized PostgreSQL SQL with no N+1 queries.
    link: /aql
    linkText: AQL reference

  - icon: ⚡
    title: Pure SQL output
    details: Axel generates SQL strings. No query executor, no connection pooling, no runtime dependency. Run the SQL however you like.

  - icon: 🔗
    title: Nested shapes, single query
    details: Select nested links in one query. Axel compiles shapes to json_agg lateral subqueries — your app receives JSON objects and arrays directly from PostgreSQL.

  - icon: 🛠️
    title: Migration lifecycle
    details: axel generate diffs your schema, axel up applies it, axel down rolls it back. Migration history is tracked in a _axel_migrations table.

  - icon: 📦
    title: Zero runtime deps
    details: The compiled SQL is plain text. Axel itself only needs a PostgreSQL connection during migrations — never during query execution.
---

## Quick look

::: code-group

```asl [schema.asl]
abstract type Base {
  required id: uuid { default := gen_uuid(); constraint pk; };
  required created_at: datetime { default := datetime_current(); };
}

type User extending Base {
  required email: str { constraint exclusive; };
  name: str;
  required age: int32;
  active: bool { default := true };
}

type Post extending Base {
  required title: str;
  required link author: User;
  multi link likes: User;
}
```

```aql [get_posts.aql]
select Post {
  id,
  title,
  author: {
    id,
    email
  },
  likes: {
    id,
    email
  }
}
filter .author.id = $author_id
order by .created_at desc
limit $limit;
```

```sql [compiled output]
-- $1: author_id
-- $2: limit
SELECT
  p.id AS id,
  p.title AS title,
  (SELECT row_to_json(u_author_sub)
   FROM (
     SELECT u_author.id AS id, u_author.email AS email
     FROM "user" u_author
     WHERE u_author.id = p.author_id LIMIT 1
   ) u_author_sub) AS author,
  (SELECT COALESCE(json_agg(row_to_json(u_likes_sub)), '[]')
   FROM (
     SELECT u_likes.id AS id, u_likes.email AS email
     FROM "post_likes" jt_likes
     JOIN "user" u_likes ON u_likes.id = jt_likes.user_id
     WHERE jt_likes.post_id = p.id
   ) u_likes_sub) AS likes
FROM "post" p
WHERE (SELECT u.id FROM "user" u WHERE u.id = p.author_id LIMIT 1) = $1
ORDER BY p.created_at DESC
LIMIT $2;
```

:::
