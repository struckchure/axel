---
title: "Inheritance — ASL"
description: "Extending one or more types"
---

# Inheritance

A type can extend one or more other types. All properties, links, indexes, and computed fields are inherited.

```asl
type User extends Timestamped {
  required email: str;
}

type Admin extends User, Audited {
  required level: int32;
}
```

> **Deprecation Notice:** The `extending` keyword is deprecated in favor of `extends`. Running `axel fmt` will automatically migrate `extending` to `extends`.

Inheriting from an [abstract type](/asl/schema/types#abstract-types) is the common way to
share a common `id` / `created_at` / `updated_at` base across many concrete
types.
