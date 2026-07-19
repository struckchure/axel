---
title: Update basics — AQL
description: The update ... set statement
---

# Update

```aql
update User
filter .id = $id
set {
  name   := $name,
  active := $active
};
```

```sql
-- $1: name
-- $2: active
-- $3: id
UPDATE "user" u SET
  name = $1,
  active = $2
WHERE u.id = $3
RETURNING *;
```

To leave columns unchanged when a value is absent, see [Partial updates](/aql/update/partial).
