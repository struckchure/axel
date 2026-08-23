---
title: "Computed Fields — ASL"
description: "Derived values expanded during AQL compilation"
---

# Computed fields

Computed fields are not stored as columns. They are expanded inline during AQL compilation.

```asl
type User {
  required email: str;
  name: str;
  computed display_name := .name ?? .email;
}

type OrderItem {
  required quantity: int32;
  required unit_price: decimal;
  discount: decimal;
  computed total := (.quantity * .unit_price) - .discount;
  computed tax := .unit_price * 0.2;
}
```

Computed fields support arithmetic operators (`+`, `-`, `*`, `/`), unary signs (`+`, `-`), function calls, and `??` (null coalescing). They can be selected in AQL shapes and queried just like ordinary fields.
