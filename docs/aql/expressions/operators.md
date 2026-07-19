---
title: Operators — AQL
description: Comparison and logical operators, and combining conditions
---

# Operators

| AQL operator | SQL equivalent |
| ------------ | -------------- |
| `=`          | `=`            |
| `!=`         | `!=`           |
| `<`          | `<`            |
| `<=`         | `<=`           |
| `>`          | `>`            |
| `>=`         | `>=`           |
| `and`        | `AND`          |
| `or`         | `OR`           |
| `??`         | `COALESCE`     |
| `in`         | `IN`           |
| `like`       | `LIKE`         |
| `ilike`      | `ILIKE`        |

## Combining conditions

Conditions chain with `and` / `or` to any length. As in SQL, **`and` binds tighter than `or`**, so
`a or b and c` means `a or (b and c)`.

```aql
multi select Project { *, members: { id } }
filter .owner = $owner<str> and .organization = $organization<str>
order by .created_at desc;
```

Parenthesize to group conditions explicitly. Groups nest to any depth:

```aql
multi select Post { id, title }
filter (.title like $q<str> or .content like $q<str>)
   and (.published = true or .author = $viewer<uuid>)
   and .deleted = false;
```

An [optional parameter](/aql/parameters/optional) inside a chain relaxes **only its own condition** —
the rest of the filter still applies. Here, omitting `$author` widens the search to every author,
but never returns an unpublished post:

```aql
multi select Post { id } filter .published = true and .author = $author<uuid>?;
```
