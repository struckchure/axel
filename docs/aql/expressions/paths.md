---
title: Path Expressions — AQL
description: .field paths and link traversal
---

# Path expressions

Paths starting with `.` refer to fields on the current type. The compiler resolves them to `alias.column`.

```aql
filter .active = true and .age >= $min_age
order by .created_at desc
```

Multi-step paths traverse a link — and chain across several — resolving to a
correlated subquery per hop:

```aql
filter .author.email = $email
filter .installation.installation_id = $iid<int64>
```

An invalid path (a step that resolves to no property or link) is a **compile
error**. For how a path's type is resolved, see [Casts & types](/aql/expressions/casts).
