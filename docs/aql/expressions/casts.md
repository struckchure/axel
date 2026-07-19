---
title: Casts & Types — AQL
description: <Type> casts and computed-field type resolution
---

# Computed field types

The type of a computed shape field is resolved in this order:

1. **Explicit cast** — a `<Type>` annotation always wins.
2. **Inferred** — a plain path is typed by resolving it through the schema: a
   path ending on a property takes that property's type; one ending on a link
   takes its FK type (`uuid`).
3. **`any`** — anything else (a coalesce, function call, arithmetic, or a path
   that can't be resolved to a scalar) is typed as `any` (`json`), and codegen
   prints a warning suggesting a cast.

So the common case needs no annotation:

```aql
multi select Application {
  *,
  owner := .project.organization.owner.id,    # inferred uuid
  iid   := .installation.installation_id      # inferred int64
}
```

A `<Type>` cast may be appended to **any operand** — a path (`.a.b<uuid>`), a
parenthesized expression (`(.name ?? .email)<str>`), a subquery projection
(`(select …).slug<str>`), or a bare literal (`'{}'<json>`). It uses the same type
names as [parameter annotations](/aql/parameters/typed), emits `(<expr>)::TYPE`, and
overrides inference / gives a type to an otherwise-uninferable field:

```aql
secrets := '{}'<json>           # a JSON literal default
who     := (.name ?? .email)<str>   # otherwise: warning + typed as any
```

An invalid path (a step that resolves to no property or link) is a **compile
error**, not a warning.
