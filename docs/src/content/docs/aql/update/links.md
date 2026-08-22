---
title: "Updating links — AQL"
description: "Reassigning a link's FK in an update set clause"
---

# Updating links

A single link can be reassigned in a `set` clause. The right-hand side is any scalar expression that
resolves to the target's FK value — the same forms accepted when [inserting a link](/aql/insert/basics).

## From a subquery

```aql
update Application
filter .id = $id<uuid>
set {
  installation := (select GithubInstallation filter .installation_id = $iid<int64>)
};
```

```sql
-- $1: iid
-- $2: id
UPDATE "application" a SET
  installation = (SELECT g.id FROM "github_installation" g WHERE g.installation_id = $1 LIMIT 1)
WHERE a.id = $2
RETURNING *;
```

## From a parameter

Pass the FK value directly. A bare link param infers `uuid`.

```aql
update Application filter .id = $id<uuid> set { owner := $owner };
```

```sql
-- $1: owner
-- $2: id
UPDATE "application" a SET owner = $1 WHERE a.id = $2 RETURNING *;
```

## Keeping the current link

Coalesce the lookup with the link's own column (`?? .link`) to leave the FK unchanged when the
subquery finds nothing. Make the lookup param optional so an omitted value produces no row and the
fallback fires — see [Optional parameters — value subquery](/aql/parameters/optional).

```aql
update Application
filter .id = $id<uuid>
set {
  installation := (select GithubInstallation filter .installation_id = $iid<int64>?) ?? .installation
};
```

```sql
-- $1: iid
-- $2: id
UPDATE "application" a SET
  installation = COALESCE(
    (SELECT g.id FROM "github_installation" g
       WHERE ($1::BIGINT IS NOT NULL AND g.installation_id = $1) LIMIT 1),
    a.installation)
WHERE a.id = $2
RETURNING *;
```

The `.installation` fallback resolves to the current row's FK column, so an omitted `$iid` keeps the
existing link instead of matching an arbitrary installation.

A subquery projection may be coalesced the same way — `(select … ).installation_id ?? .installation_id`
selects the linked FK column rather than the row id, and an optional cast (`.field<str>`) applies to
the projected value.

---

## Multi-links

Many-to-many relationships (`multi link members: User`) can be modified using either delta assignments (`{ "+": ..., "-": ... }`) or full set replacement.

### Delta modification (`+` and `-`)

Use `"+"` to add items and `"-"` to remove items from a multi-link:

```aql
update Organization
filter .id = $id<uuid>
set {
  members := {
    "+": (multi select User filter .email in $invite_emails),
    "-": (select User filter .id = $removed_user_id<uuid>)
  }
};
```

This compiles to a clean CTE pipeline that applies the deletions and insertions on the underlying junction table:

```sql
WITH _target AS (
  SELECT o.* FROM "organization" o
  WHERE o.id = $1
),
_del_members AS (
  DELETE FROM "organization_members"
  WHERE "organization" IN (SELECT id FROM _target)
    AND "user" IN (SELECT u.id FROM "user" u WHERE u.id = $2)
),
_ins_members AS (
  INSERT INTO "organization_members" ("organization", "user")
  SELECT _target.id, _sub.id
  FROM _target
  CROSS JOIN (SELECT u.id FROM "user" u WHERE u.email IN (...)) AS _sub(id)
  ON CONFLICT DO NOTHING
)
SELECT o.id, o.name FROM _target o;
```

- Removals (`"-"`) always execute before additions (`"+"`).
- Either `"+"` or `"-"` or both can be provided.
- Keys can be written as `"+"` / `"-"`, `'+'` / `'-'`, or bare `+` / `-`.

### Full replacement

Assigning an expression directly to a multi-link reconciles the relation by replacing all existing links with the new set:

```aql
update Organization
filter .id = $id<uuid>
set {
  members := (multi select User filter .department = 'Engineering')
};
```

```sql
WITH _target AS (
  SELECT o.* FROM "organization" o
  WHERE o.id = $1
),
_del_members AS (
  DELETE FROM "organization_members"
  WHERE "organization" IN (SELECT id FROM _target)
),
_ins_members AS (
  INSERT INTO "organization_members" ("organization", "user")
  SELECT _target.id, _sub.id
  FROM _target
  CROSS JOIN (SELECT u.id FROM "user" u WHERE u.department = 'Engineering') AS _sub(id)
  ON CONFLICT DO NOTHING
)
SELECT o.id, o.name FROM _target o;
```

