---
title: Soft deletes — Examples
description: Hide deleted rows with a policy instead of removing them
---

# Soft deletes

Instead of physically deleting a row, stamp a `deleted_at` and hide it from reads.
A `Soft` mixin carries the column and a [policy](/asl/policies) that filters
deleted rows out of every `SELECT`:

```asl
abstract type Soft {
  deleted_at: datetime;
  policy not_deleted for select using ( .deleted_at is null );
}

type Article extending Base, Soft {
  required title: str;
}
```

"Delete" is an update that sets the timestamp:

```aql
update Article filter .id = $id<uuid> set { deleted_at := now() };
```

Reads through the app role only ever see live rows — the policy appends
`WHERE deleted_at IS NULL` for you:

```aql
multi select Article { id, title };
```

Counting explicitly (e.g. from a privileged role that bypasses RLS) still works
with an `is null` filter:

```aql
select count(Article filter .deleted_at is null);
```

## From generated code

Save the three queries as `soft_delete_article.aql`, `list_articles.aql`, and
`count_live_articles.aql`:

::: code-group

```ts [TypeScript]
await runner.query.softDeleteArticle({ id });   // stamps deleted_at
const live = await runner.query.listArticles(); // ListArticlesRow[] — deleted rows hidden by the policy
const n = await runner.query.countLiveArticles(); // number
```

```go [Go]
_, err := runner.Query.SoftDeleteArticle(ctx, gen.SoftDeleteArticleParams{ID: id})
live, err := runner.Query.ListArticles(ctx) // []ListArticlesRow
n, err := runner.Query.CountLiveArticles(ctx) // int64
```

:::

::: tip
The policy applies to ordinary roles; the **table owner bypasses RLS**, so a
privileged cleanup job can still see and purge soft-deleted rows. See
[Policies → the table owner bypasses RLS](/asl/policies).
:::
