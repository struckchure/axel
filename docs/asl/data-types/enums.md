---
title: Enums — ASL
description: Enumerated string types with a CHECK constraint
---

# Enum types

Enums are stored as `TEXT` with a named `CHECK (col IN (...))` constraint (`chk_<table>_<col>_enum`)
restricting the column to the declared values.

```asl
enum Role { Admin, Member, Guest }
```

Use an enum as a property type. Reference an enum value in a default with the qualified
`Enum.Member` form (a quoted literal `'Member'` is also accepted); the value is validated against the
enum's declaration at `generate` time:

```asl
type User {
  required role: Role { default := Role.Member; };
}
```

This emits `"role" TEXT NOT NULL DEFAULT 'Member' CONSTRAINT "chk_user_role_enum" CHECK ("role" IN ('Admin', 'Member', 'Guest'))`.
Generated Go/TS code uses the enum type for the field (`Role`) rather than a plain string — for
model structs, query **parameters**, and query **result columns** alike (including columns pulled in
by a `*` splat and inside nested sub-select rows).
