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

A link assignment accepts any scalar expression that resolves to the FK value, not just a solo
subquery:

- **A bare parameter** — pass the FK directly; a lone link param infers `uuid`.

  ```aql
  insert Post { title := $title, author := $author_id };
  ```

- **A subquery projection** — select a *linked* FK column rather than the row id with `(select …).link`.

  ```aql
  insert GithubInstallation {
    organization    := (select GithubInstallation filter .installation_id = $iid<int64>).organization,
    installation_id := $iid<int64>
  };
  ```

- **A `??` chain** — coalesce several lookups; the FK resolves from whichever finds a row first.

  ```aql
  insert GithubInstallation {
    organization    := (select Organization filter .id = $org<uuid>?)
                     ?? (select GithubInstallation filter .installation_id = $iid<int64>?).organization,
    installation_id := $iid<int64>
  };
  ```

  See [Optional parameters — value subquery](/aql/parameters/optional) for how an omitted param lets
  the chain fall through.

- **A sub-insert** — create the linked row inline; it lowers to a CTE. See [Conflicts](/aql/insert/conflicts)
  for the (unsupported) interaction with `unless conflict` on sub-inserts.

  ```aql
  insert Post {
    title  := $title,
    author := (insert User { email := $email, name := $name })
  };
  ```

To handle a uniqueness collision, see [Conflicts](/aql/insert/conflicts). To reassign a link on an
existing row, see [Updating links](/aql/update/links).
