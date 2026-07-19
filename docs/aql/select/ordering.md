---
title: Ordering & Pagination — AQL
description: order by, limit, and offset
---

# Ordering & Pagination

## Order by

```aql
select User { id, email }
order by .created_at desc;
```

```sql
SELECT u.id AS id, u.email AS email
FROM "user" u
ORDER BY u.created_at DESC
LIMIT 1;
```

Multiple fields:

```aql
select User { id, email }
order by .active desc, .created_at asc;
```

## Limit and offset

`limit`/`offset` require `multi select` (a plain select already returns a single row).

```aql
multi select User { id, email }
order by .created_at desc
limit $limit
offset $offset;
```

```sql
-- $1: limit
-- $2: offset
SELECT u.id AS id, u.email AS email
FROM "user" u
ORDER BY u.created_at DESC
LIMIT $1
OFFSET $2;
```

## Combining clauses

```aql
multi select User { id, email, name }
filter .active = true and .age >= $min_age
order by .created_at desc
limit $limit
offset $offset;
```
