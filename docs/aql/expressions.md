---
title: Expressions — AQL
description: Operators, literals, path expressions, and casts
---

# Expressions

- **[Operators](/aql/expressions/operators)** — comparison / logical operators, `is null`, and combining conditions.
- **[Literals](/aql/expressions/literals)** — string, number, boolean, and null literals.
- **[Path Expressions](/aql/expressions/paths)** — `.field` paths and link traversal.
- **[Casts & Types](/aql/expressions/casts)** — `<Type>` casts and computed-field type resolution.

## Global references

`global <name>` reads a declared [global variable](/asl/globals) — the current
user, active tenant, etc. It's an operand like a path or literal, so it works
anywhere an expression does (a `filter`, a computed field, an RLS
[policy](/asl/policies) predicate):

```aql
multi select Doc { id, title } filter .owner = global current_user;
```

It lowers to a read of the backing session setting,
`current_setting('app.<name>', …)`. See [Globals](/asl/globals) for declaration
and how the value is set from the client.
