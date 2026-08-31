---
title: "Properties — ASL"
description: "Scalar columns, required, and defaults"
---

# Properties

Properties map to columns.

```asl
type User {
  required email: str;           # NOT NULL column
  name: str;                     # nullable column
  required property age: int32;  # "property" keyword is optional
}
```

## Required

`required` maps to `NOT NULL`.

## Defaults

```asl
active: bool { default := true };
name: str    { default := 'anonymous'; };
score: int32 { default := 0; };

# Functions
id:         uuid     { default := gen_uuid(); };
created_at: datetime { default := datetime_current(); };
```

A `default` runs once, on INSERT. To re-assign a field on later updates, see
[Rewrites](/asl/fields/rewrites).

Declaring a default also matters when you **add** a required field to a table that already has rows:
with one, Axel backfills existing rows itself; without one, the migration leaves a seam you must
fill in. See [`axel diff`](/cli#axel-diff).

## Multi (array) properties

`multi` on a property declares a PostgreSQL **array column** — one column, not a junction table.
(`multi` on a *link* means something else entirely; see [Links](/asl/fields/links).)

```asl
type User {
  multi roles: UserType;    # TEXT[] with a containment check
  multi images: str;        # TEXT[]
  multi scores: float64;    # DOUBLE PRECISION[]
}
```

```sql
"roles"  TEXT[] CONSTRAINT "chk_user_roles_enum" CHECK ("roles" <@ ARRAY['Admin', 'Runner', 'Vendor']::TEXT[]),
"images" TEXT[],
"scores" DOUBLE PRECISION[]
```

An enum array is stored as `TEXT[]` guarded by a containment check rather than as a Postgres enum
array.

Two consequences worth knowing:

- **Membership compiles to `= ANY(...)`**, not `IN` — Postgres `IN` takes a parenthesised list and
  rejects an array operand.

  ```aql
  select User { id } filter UserType.Admin in .roles;   # → 'Admin' = ANY(u.roles)
  ```

- **Generated clients type it as an array**, with the nullability outside the array:
  `roles?: UserType[] | null` in TypeScript, `[]UserType` in Go — in the models and in query result
  rows alike.
