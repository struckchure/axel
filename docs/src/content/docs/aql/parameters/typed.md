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

### `:type` and `<type>` are the same

A declaration may spell its type either way. Pick one and stay consistent within a query:

```aql
var (
  $status: TransactionStatus?;    # same as $status<TransactionStatus>?
  $limit<int32> := 20;            # same as $limit: int32 := 20
)
```

The optional `?` goes after the type, and a `:= default` after that.

## Array parameters (`multi`)

Prefix a declaration with `multi` to bind a **single array value** rather than one element:

```aql
var (
  multi $ids: uuid;
  multi $roles: UserType;
)

multi select User { id, email }
filter .id in $ids and .role in $roles;
```

```sql
-- $1: ids (uuid)
-- $2: roles (str)
SELECT u.id AS id, u.email AS email
FROM "user" u
WHERE u.id = ANY($1::UUID[]) AND u.role = ANY($2::TEXT[]);
```

Membership against an array parameter lowers to `= ANY($n::T[])`, never `IN` — Postgres `IN` expects
a parenthesised list and rejects an array bind.

The parameter reaches the generated clients as an array type — **one argument, not a spread**:

```ts
await queries.usersByIds({ ids: ["…", "…"], roles: [UserType.Admin] });
```

```go
rows, err := queries.UsersByIds(ctx, gen.UsersByIdsParams{
    Ids:   []string{"…", "…"},
    Roles: []gen.UserType{gen.UserTypeAdmin},
})
```

If the declaration omits the type, the element type is inferred from the column the parameter is
compared against:

```aql
var ( multi $ages; )
multi select User { id } filter .age in $ages;    # $ages inferred int32, → u.age = ANY($1)
```

An array parameter may also carry a default, which is what [bulk inserts](/aql/insert/bulk) iterate
over:

```aql
var multi $conditions: str? := {'Hot', 'Cold', 'Fragile'};
```

Array parameters are distinct from `multi` **fields** in the schema, though they interact naturally:
a `multi` scalar column is also compared with `= ANY`. See
[Multi properties](/asl/fields/properties).

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
- a **scalar alias** — e.g. `scalar type EmailStr extends str` renders as its base builtin
- an **enum** — e.g. `TransactionStatus`, which generates the real enum type in code (Go `TransactionStatus`, TypeScript `TransactionStatus`) rather than a bare `string`

Object types (tables) are **not** valid parameter types — a parameter is a value, not a row — and an unknown type name is a compile error.

Annotations override inference, so an explicit annotation always wins. Even without one, an enum-backed column is inferred as its enum type: `filter .status = $status` types `$status` as `TransactionStatus` automatically.
