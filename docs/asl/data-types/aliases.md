---
title: Aliases — ASL
description: Named scalar aliases over a built-in scalar
---

# Named scalar aliases

Create a named alias over a built-in [scalar](/asl/data-types/scalars).

```asl
scalar type EmailStr extending str;
scalar type Score extending float32;
```

Use aliases like any other type:

```asl
type User {
  required email: EmailStr;
}
```
