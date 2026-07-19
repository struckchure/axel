---
title: Indexes — ASL
description: Index declarations on one or more columns
---

# Indexes

```asl
type User {
  required email: str;
  required age: int32;
  active: bool;

  index on (.email);
  index on (.active, .age);
}
```

Each `index on (...)` declaration generates a `CREATE INDEX` statement in the migration SQL.
