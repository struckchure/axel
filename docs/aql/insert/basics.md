---
title: Insert basics — AQL
description: Inserting rows and assigning links
---

# Insert

```aql
insert User {
  email := $email,
  name  := $name,
  age   := $age
};
```

```sql
-- $1: email
-- $2: name
-- $3: age
INSERT INTO "user" ("email", "name", "age")
VALUES ($1, $2, $3)
RETURNING *;
```

## Inserting with a link

Assign a link by passing a subquery that resolves to the FK value.

```aql
insert Post {
  title  := $title,
  author := (select User filter .email = $email)
};
```

```sql
-- $1: title
-- $2: email
INSERT INTO "post" ("title", "author_id")
VALUES ($1, (SELECT u.id FROM "user" u WHERE u.email = $2 LIMIT 1))
RETURNING *;
```

To handle a uniqueness collision, see [Conflicts](/aql/insert/conflicts).
