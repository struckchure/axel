---
title: Conflicts (unless conflict) — AQL
description: ON CONFLICT do-nothing and upsert behavior
---

# Handling conflicts (`unless conflict`)

An `insert` may declare what to do when it collides with an existing row on a
unique (`exclusive`) or primary-key constraint. This lowers to Postgres
`ON CONFLICT`.

**Do nothing on any conflict:**

```aql
insert User { email := $email, name := $name } unless conflict;
```

```sql
INSERT INTO "user" ("email", "name")
VALUES ($1, $2)
ON CONFLICT DO NOTHING
RETURNING *;
```

**Do nothing on a specific constraint:**

```aql
insert User { email := $email, name := $name } unless conflict on .email;
```

```sql
INSERT INTO "user" ("email", "name")
VALUES ($1, $2)
ON CONFLICT ("email") DO NOTHING
RETURNING *;
```

Use `on (.a, .b)` to target a composite `exclusive` constraint.

**Upsert — update the existing row on conflict (`else`):**

```aql
insert User { email := $email, name := $name }
unless conflict on .email
else (update User set { name := $name });
```

```sql
INSERT INTO "user" ("email", "name")
VALUES ($1, $2)
ON CONFLICT ("email") DO UPDATE SET "name" = $2
RETURNING *;
```

Rules and behavior:

- The `on` target must be backed by an `exclusive` or primary-key constraint;
  otherwise compilation fails.
- `else` requires an `on` target, its type must match the insert's type, and it
  takes no `filter` (Postgres targets the conflicting row automatically).
- **`RETURNING` behavior differs by form:** `DO UPDATE` returns the updated row,
  but `DO NOTHING` returns **no row** when a conflict occurs. Handle the empty
  result in calling code for the `unless conflict` / `unless conflict on ...`
  forms.
- The clause is supported on top-level inserts only (not nested `(insert ...)`
  link sub-inserts).
