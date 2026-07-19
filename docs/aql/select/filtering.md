---
title: Filtering — AQL
description: The filter clause on a select
---

# Filtering

```aql
select User { id, email }
filter .active = true and .age >= $min_age;
```

```sql
-- $1: min_age
SELECT u.id AS id, u.email AS email
FROM "user" u
WHERE u.active = true AND u.age >= $1
LIMIT 1;
```

See [Expressions](/aql/expressions) for the full set of operators and how conditions combine with `and` / `or`.
