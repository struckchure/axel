---
title: Audit timestamps & UUID keys — Examples
description: A reusable Base type with UUID primary keys and auto-updated timestamps
---

# Audit timestamps & UUID keys

A `Base` abstract type gives every table a UUID primary key, a `created_at` set
once on insert, and an `updated_at` that touches itself on every update. Types
that `extend Base` inherit all three.

```asl
abstract type Base {
  required id: uuid {
    default := gen_uuid();
    constraint exclusive;
    constraint pk;
  };
  required created_at: datetime { default := datetime_current(); };
  required updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();
  };
}

type Article extending Base {
  required title: str;
}
```

- `gen_uuid()` / `datetime_current()` are Axel builtins that lower to
  `gen_random_uuid()` (via [pgcrypto](/asl/extensions)) and `now()`.
- The `rewrite update := datetime_current()` on `updated_at` becomes a
  `BEFORE UPDATE` trigger, so the column is stamped in the database — you never
  set it from application code. See [Rewrites](/asl/fields/rewrites).

`Article` inherits the columns and the trigger; a plain insert only needs
`title`:

```aql
insert Article { title := $title<str> };
```

## From generated code

Save that query as `create_article.aql` and [`axel codegen`](/codegen) produces a
typed `createArticle` function (and a `runner.query.createArticle` method). The
returned row carries the DB-populated `id`, `createdAt`, and `updatedAt`:

::: code-group

```ts [TypeScript]
const article = await runner.query.createArticle({ title: "Hello" });
// article.id, article.createdAt, article.updatedAt are set by the database
```

```go [Go]
article, err := runner.Query.CreateArticle(ctx, gen.CreateArticleParams{Title: "Hello"})
// article.ID, article.CreatedAt, article.UpdatedAt are set by the database
```

:::

Because `Base` is `abstract`, it produces no table of its own — it's a mixin you
add to any concrete type ([Inheritance](/asl/schema/inheritance)).
