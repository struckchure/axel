---
title: With — AQL
description: Bind a subquery once with `with (...)` and reuse it across the query
---

# With

A `with (...)` block binds named subqueries for the statement that follows. Each binding
lowers to a Postgres CTE, so a sub-select used at several points in a filter is evaluated
once instead of being inlined per use site.

```aql
with (
  business := (select Business filter .id = $business_id);
  api_keys := (multi select ApiKey filter .business = $business_id);
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

## Narrowing the projection with `{ shape }`

By default a binding projects every scalar column and single-link FK column of its type
into the CTE. When only a subset of fields will be used at the reference sites, you can
list them explicitly with a `{ shape }`:

```aql
with (
  api_key := (
    multi select ApiKey { id }
    filter .id = $api_key_id<uuid>? or .business.id = $business_id<uuid>?
  )
)
multi select Transaction
filter .sender_id in api_key.id or .reciever_id in api_key.id;
```

Only the named fields are projected into the CTE. Referencing a field that was not
included in the shape is caught at compile time:

```aql
-- error: field "label" was not included in the { shape }
filter .name = api_key.label
```

Use `*` to keep the full projection explicit without restricting it:

```aql
api_key := (multi select ApiKey { * } filter ...)
```

### Casting on a set reference

When the column type in the CTE does not match the column you are comparing against,
you can attach a `<cast>` directly to the field reference on the right of `in`. The cast
is applied inside the subquery projection:

```aql
-- api_key.id is uuid; sender_id is stored as text — cast at the reference site
filter .sender_id in api_key.id<str> or .reciever_id in api_key.id<str>
```

This avoids having to rewrite the binding itself or add a computed column.

## Restrictions

`limit` / `offset` require a `multi` binding, matching the rule for a plain `select`.
Aggregates cannot be bound; use them directly in the query. A `with` block is also not
available inside a trigger or function body, which has no host statement to carry the CTE.

Nested sub-shapes and computed (`:=`) fields inside a binding shape are not supported —
the shape may only name scalar properties and single-link FK columns.

## Formatting

`axel fmt` keeps each binding on its own line. When a binding subquery doesn't fit on a
single line, it is wrapped in indented parens with each clause on its own line — the same
treatment a top-level select gets:

```aql
with (
  api_key := (
    multi select ApiKey { id }
    filter .id = $api_key_id<uuid>? or .business.id = $business_id<uuid>?
  )
)
```

Short bindings that fit within 80 columns are left on a single line:

```aql
with (business := (select Business filter .id = $business_id))
```

Boolean filters are broken across lines only when the single-line form exceeds 80 columns.
The formatter never adds or removes parentheses, so the grouping you write is the grouping
you keep.
