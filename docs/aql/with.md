---
title: With bindings — AQL
description: Bind a subquery once with `with (...)` and reuse it across the query
---

# With bindings

A `with (...)` block binds named subqueries for the statement that follows. Each binding
lowers to a Postgres CTE, so a sub-select used at several points in a filter is evaluated
once instead of being inlined per use site.

```aql
with (
  business := (select Business filter .id = $business_id),
  api_keys := (multi select ApiKey filter .business = $business_id)
)
multi select Transaction
filter (
  business is not null
  and (
    .sender_id = business.id
    or .sender_id in api_keys.id
    or .reciever_id in api_keys.id
  )
)
order by .updated_at desc
limit $limit<int32>?
offset $offset<int32>?;
```

A block may precede any statement — `select`, `insert`, `update`, or `delete`.

## Single-row and set bindings

The `multi` keyword decides what a binding is, exactly as it does on a select:

- `name := (select T ...)` binds a **single row**. It is capped at one row, and referencing
  it yields a value.
- `name := (multi select T ...)` binds a **set** of rows. It is not capped, and it is only
  usable on the right of `in`.

Using a set binding as a value is rejected at compile time, rather than becoming a
`more than one row returned by a subquery` failure at run time:

```aql
-- error: binding "api_keys" is a `multi select` (a set, not a value)
filter .sender_id = api_keys.id
```

## Referring to a binding

A binding is referenced by name, in two forms:

| Form | Meaning |
| --- | --- |
| `business` | the bound row's `id` |
| `business.id`, `business.name` | that column of the bound row |

The bare form is what makes an existence test read naturally:

```aql
filter business is not null
```

A binding **shadows a type or enum of the same name** for the whole statement, so
`Business.id` inside the query below reads the binding, not the type:

```aql
with (Business := (select Business filter .slug = $slug))
multi select User filter .business = Business.id;
```

Lowercase binding names avoid the ambiguity entirely and are the recommended style.

## Restrictions

A binding names a whole row, so it takes neither a shape nor a projection — select the
columns you want at the use site:

```aql
-- error: a with-binding selects every column of Business
with (business := (select Business { id } filter .id = $id))

-- error: project at the use site instead
with (slug := (select Business filter .id = $id).slug)
```

`limit` / `offset` require a `multi` binding, matching the rule for a plain `select`.
Aggregates cannot be bound; use them directly in the query. A `with` block is also not
available inside a trigger or function body, which has no host statement to carry the CTE.

## Formatting

`axel fmt` keeps each binding on its own line and breaks a boolean filter across lines when
the single-line form would exceed 80 columns — the layout shown at the top of this page is
what the formatter produces. Shorter filters are left inline:

```aql
multi select User filter .active = true and .age >= $min_age;
```

The formatter never adds or removes parentheses, so the grouping you write is the grouping
you keep.
