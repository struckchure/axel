---
title: "Typed parameters — AQL"
description: "$name<type> annotations and type inference"
---

# Typed parameters

By default a parameter's type is **inferred** from the property it's compared against (`.email = $email` → `str`) or the column it's assigned to. Params with no such anchor — most commonly `limit` / `offset` — have no inferable type and would otherwise generate a loose `any` field.

## Top-level `var` declarations

You can declare and type parameters at the top of your query using a `var (...)` block or individual `var` statements:

```aql
var (
  $status<TransactionStatus>?;
  $limit<int32>?;
  $offset<int32>?;
)

multi select Transaction { id }
filter .status = $status
order by .created_at desc
limit $limit
offset $offset;
```

Or as single `var` statements:

```aql
var $status<TransactionStatus>?;
var $limit<int32>?;

multi select Transaction { id } filter .status = $status limit $limit;
```

When declared with `var`, parameters can be referenced as bare names (`$status`, `$limit`) throughout filters, subqueries, and clauses without needing inline `<type>` annotations repeated at every use site.

## Inline annotations

Alternatively, annotate a parameter inline where it is used with `$name<type>`. The annotation goes before any `?`:

```aql
multi select Transaction { id }
filter .status = $status<TransactionStatus>
order by .created_at desc
limit $limit<int32>?
offset $offset<int32>?;
```

The type may name any declared **value** type from your schema:

- a **builtin scalar** — `str`, `int16`/`int32`/`int64`, `float32`/`float64`, `bool`, `uuid`, `datetime`, `date`, `time`, `json`, `bytes`, `decimal`
- a **scalar alias** — e.g. `scalar type EmailStr extending str` renders as its base builtin
- an **enum** — e.g. `TransactionStatus`, which generates the real enum type in code (Go `TransactionStatus`, TypeScript `TransactionStatus`) rather than a bare `string`

Object types (tables) are **not** valid parameter types — a parameter is a value, not a row — and an unknown type name is a compile error.

Annotations override inference, so an explicit annotation always wins. Even without one, an enum-backed column is inferred as its enum type: `filter .status = $status` types `$status` as `TransactionStatus` automatically.
