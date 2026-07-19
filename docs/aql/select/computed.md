---
title: Computed shape fields — AQL
description: Inline expressions and sub-selects in a shape
---

# Computed shape fields

A shape field can be assigned an inline expression using `:=`. A common use is a sub-select that pulls related data without defining a link in the schema.

A sub-select follows the same cardinality rule as a top-level query: **plain `select` returns a single object** (or `null`), and **`multi select` returns a JSON array**.

```aql
select User {
  id,
  email,
  posts := (multi select Post { id, title } filter .author.id = User.id),  # array
  primary_org := (select Organization { id, name } filter .owner = User.id) # single object or null
}
```

A `multi select` compiles to a correlated `json_agg` subquery (empty array — never null — when nothing matches); a plain `select` compiles to `row_to_json` over a `LIMIT 1` inner query (null when nothing matches). The outer type name (`User.id`) is a **qualified reference** — it resolves to the outer query's alias.

```sql
  (SELECT COALESCE(json_agg(row_to_json(p_posts_sub)), '[]')
   FROM (SELECT p.id AS id, p.title AS title FROM "post" p WHERE p.author = u.id) p_posts_sub) AS posts,
  (SELECT row_to_json(o_primary_org_sub)
   FROM (SELECT o.id AS id, o.name AS name FROM "organization" o WHERE o.owner = u.id LIMIT 1) o_primary_org_sub) AS primary_org
```

Computed shape fields with no sub-select compile as scalar expressions:

```aql
select User {
  id,
  label := .name ?? .email
}
```

## Projecting a field from a subquery

A subquery normally resolves to a row's id. Append `.field` to project a single
column instead — the subquery then behaves as a scalar and can be combined with
other operators. This works anywhere an expression is allowed, including
`insert` / `update` assignment values:

```aql
update Repo filter .id = $id<uuid> set {
  installation_id :=
    (select GithubInstallation filter .id = $installation_id<uuid>?).installation_id
      ?? .installation_id
}
```

```sql
UPDATE "repo" r SET
  installation_id = COALESCE(
    (SELECT g.installation_id FROM "github_installation" g
     WHERE ($1::UUID IS NULL OR g.id = $1) LIMIT 1),
    r.installation_id)
WHERE r.id = $2
```

The projected field must be a scalar property or a link on the subquery's type;
an unknown field is a compile error.

An optional `<Type>` after the projection casts the result (see
[Casts & types](/aql/expressions/casts) — the cast works on any operand):

```aql
installation_id := (select GithubInstallation filter .id = $id<uuid>).installation_id<str> ?? .installation_id
```

```sql
installation_id = COALESCE(((SELECT g.installation_id FROM ... LIMIT 1))::TEXT, r.installation_id)
```

> **Note:** field projection is available in generated queries (both Go and
> TypeScript output, which share the compiler). The TypeScript runtime `aql`
> tagged-template — for queries assembled dynamically at runtime — does not yet
> parse projections or `??` in assignment values.
