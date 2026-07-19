---
title: Properties — ASL
description: Scalar columns, required, and defaults
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
