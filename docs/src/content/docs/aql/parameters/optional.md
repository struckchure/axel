---
title: "Optional parameters — AQL"
description: "$name? and how it behaves in filters vs set clauses"
---

# Optional parameters

A trailing `?` marks a parameter optional (`$email?`). In a filter, an optional parameter is **skipped when its value is null** — the condition becomes a no-op — so a single query can support present/absent filters. The generated parameter type is nullable (Go `*T`, TypeScript `field?: T | null`).

```aql
multi select User { id, email }
filter .email = $email?;
```

The same optionality can be declared once in a [`var` block](/aql/parameters/typed) instead of at
every use site — the two forms compile identically:

```aql
var ( $email: str?; )
multi select User { id, email } filter .email = $email;
```

```sql
-- $1: email
SELECT u.id AS id, u.email AS email
FROM "user" u
WHERE ($1 IS NULL OR u.email = $1);
```

Passing `null` for `email` returns all users; passing a value filters by it.

## In an `or` group

The skip-when-null behavior above is the identity of an **`and`** context: an omitted param matches
every row, so the surrounding conjunction is unaffected. Inside an **`or`**, that same "match-all"
would satisfy the whole disjunction and silently void the other arms. So an omitted optional param in
an `or` instead **drops out** of the group — the guard flips from `IS NULL OR` to `IS NOT NULL AND`:

```aql
multi select Project
filter .owner = $owner? or .organization = $org?;
```

```sql
-- $1: owner
-- $2: organization
SELECT ...
FROM "project" p
WHERE ($1::UUID IS NOT NULL AND p.owner = $1)
   OR ($2::TEXT IS NOT NULL AND p.organization = $2);
```

Each optional relaxes only its own comparison; the connective it sits in decides whether "omitted"
means match-all (`and`) or drop-out (`or`). In a mixed expression the arms inside a parenthesized
`or` group take the drop-out identity while a sibling optional filter outside the group keeps
match-all.

## Inside a value subquery

When a scalar subquery is used as a *value* — a link assignment or a `(select ...)` operand — an
omitted optional param in its filter must yield **no row** (so the subquery evaluates to `NULL` and a
`??` fallback can fire), rather than matching all rows and returning an arbitrary one. The value
context forces the same `IS NOT NULL AND` guard:

```aql
insert GithubInstallation {
  organization := (select Organization filter .id = $org<uuid>?)
               ?? (select GithubInstallation filter .installation_id = $iid<int64>?).organization,
  installation_id := $iid<int64>
};
```

```sql
COALESCE(
  (SELECT o.id FROM "organization" o
     WHERE ($1::UUID IS NOT NULL AND o.id = $1) LIMIT 1),
  (SELECT g.organization FROM "github_installation" g
     WHERE ($2::BIGINT IS NOT NULL AND g.installation_id = $2) LIMIT 1))
```

When `$org` is omitted the first lookup returns nothing, so the `??` chain falls through to the
second. See [Insert basics](/aql/insert/basics) and [Updating links](/aql/update/links).

## With a default

A parameter declared with a default is **coalesced, not skipped** — the comparison still runs, using
the default when the value arrives null. Skipping it as well would silently ignore the default you
asked for:

```aql
var ( $age: int32? := 21; )
multi select User { id } filter .age >= $age;
```

```sql
WHERE u.age >= COALESCE($1::INTEGER, 21)
```

## Optional array parameters

An optional [`multi` parameter](/aql/parameters/typed#array-parameters-multi) is guarded the same
way, and every cast of the placeholder stays the **array** type:

```aql
var ( multi $emails: str?; )
multi select User { id } filter .email in $emails;
```

```sql
WHERE ($1::TEXT[] IS NULL OR u.email = ANY($1::TEXT[]))
```

Casting one placeholder to both `TEXT` and `TEXT[]` in the same statement would make Postgres reject
it, so the array type wins everywhere.

## In an `update` `set` clause

An optional parameter assigned directly to a column behaves differently again — `null` writes `NULL`
to the column rather than being skipped. See [Partial updates](/aql/update/partial).
