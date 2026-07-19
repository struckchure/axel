---
title: Aggregates — AQL
description: count and aggregate selects
---

# Aggregate select

## count

```aql
select count(User);
```

```sql
SELECT COUNT(*) FROM (
  SELECT 1 FROM "user" u
) _agg;
```

With a filter:

```aql
select count(User filter .active = true);
```

```sql
SELECT COUNT(*) FROM (
  SELECT 1 FROM "user" u
  WHERE u.active = true
) _agg;
```
