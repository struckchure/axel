---
title: Inheritance — ASL
description: Extending one or more types
---

# Inheritance

A type can extend one or more other types. All properties, links, indexes, and computed fields are inherited.

```asl
type User extending Timestamped {
  required email: str;
}

type Admin extending User, Audited {
  required level: int32;
}
```

The keyword `extends` is accepted as a synonym for `extending`.

Inheriting from an [abstract type](/asl/schema/types#abstract-types) is the common way to
share a common `id` / `created_at` / `updated_at` base across many concrete
types.
