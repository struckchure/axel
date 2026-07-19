---
title: Delete — AQL
description: Deleting rows
---

# Delete

```aql
delete User filter .id = $id;
```

```sql
-- $1: id
DELETE FROM "user" u
WHERE u.id = $1;
```
