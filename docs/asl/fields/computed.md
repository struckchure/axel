---
title: Computed Fields — ASL
description: Derived values expanded during AQL compilation
---

# Computed fields

Computed fields are not stored as columns. They are expanded inline during AQL compilation.

```asl
type User {
  required email: str;
  name: str;
  computed display_name := .name ?? .email;
}
```

Use `??` for a null-coalescing fallback. Computed fields can be referenced in AQL shapes.
