---
title: Types — ASL
description: Concrete and abstract types
---

# Types

## Concrete types

A concrete type maps to a database table.

```asl
type User {
  required email: str;
  name: str;
  required age: int32;
}
```

The keyword `model` is accepted as a synonym for `type`.

## Abstract types

Abstract types have no table of their own. They exist only to be extended by other types.

```asl
abstract type Timestamped {
  required id: uuid {
    default := gen_uuid();
    constraint pk;
  };
  required created_at: datetime { default := datetime_current(); };
  required updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();   # keep it fresh on every UPDATE
  };
}
```

> `default` only fires on INSERT, so without the `rewrite` line `updated_at` would
> never change. See [Rewrites](/asl/fields/rewrites).

Abstract types are meant to be reused through [inheritance](/asl/schema/inheritance).
