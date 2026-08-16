---
title: "Policies (RLS) — ASL"
description: "Row-level security policies attached to a type"
---

# Policies

A `policy` inside a type body declares a Postgres **row-level security** policy on
that table. Axel emits a `CREATE POLICY` and enables RLS on the table
(`ALTER TABLE … ENABLE ROW LEVEL SECURITY`). Policies filter which rows a role
can read or write — the classic use being to hide rows a query shouldn't see.

```asl
type KV {
  required key: str { constraint exclusive; };
  required value: json;
  expires_at: datetime;

  # Hide rows past their TTL from SELECT (visible = not-yet-expired)
  policy hide_expired for select
    using ( .expires_at is null or .expires_at >= now() );
}
```

lowers to:

```sql
ALTER TABLE "kv" ENABLE ROW LEVEL SECURITY;
CREATE POLICY "hide_expired" ON "kv"
  FOR SELECT USING (expires_at IS NULL OR expires_at >= now());
```

## Syntax

```
policy <name> for <command>, … [to <role>, …] [using ( … )] [with check ( … )];
```

- **`for <command>, …`** — one or more of `select`, `insert`, `update`, `delete`,
  or `all`. Postgres allows a single command per `CREATE POLICY`, so a list like
  `for update, delete` **expands to one policy per command** — the generated
  policies are suffixed (`<name>_update`, `<name>_delete`) to keep their names
  unique. A single-command policy keeps its declared name.
- **`to <role>, …`** — the roles the policy applies to. Omit for `PUBLIC`.
- **`using ( … )`** — predicate for **existing** rows: which rows are visible to
  `SELECT`/`UPDATE`/`DELETE`. A row is visible when the predicate is true.
- **`with check ( … )`** — predicate for **new/updated** rows on
  `INSERT`/`UPDATE`; a write is rejected when it's false.

At least one of `using` / `with check` is required.

:::caution[Which clause each command accepts]
Postgres restricts the clauses per command, and Axel validates this at
`axel validate` / `axel diff` time (before it ever reaches the database):

- `using` — `select`, `update`, `delete`, `all`
- `with check` — `insert`, `update`, `all`

So `policy p for delete with check ( … )` is rejected up front (a `DELETE` has no
new row to check) — use `using ( … )` to block deletes instead. See the
[append-only log example](/examples/append-only-log).
:::

A multi-command policy lowers to one statement per command:

```asl
type Event {
  required topic: str;
  policy append_only for update, delete using ( false );
}
```

```sql
CREATE POLICY "append_only_update" ON "event" FOR UPDATE USING (false);
CREATE POLICY "append_only_delete" ON "event" FOR DELETE USING (false);
```

## The predicate

Predicates are **native [AQL](/aql/) expressions** — the same language used in
query `filter` clauses — resolved and type-checked against the type. A `.field`
reference resolves to that field's column; `and`/`or`, comparisons, `is null` /
`is not null`, `??`, casts (`<uuid>`), and function calls (`now()`,
`current_user`) all work as in any AQL filter.

```asl
global current_user: uuid;

type Doc {
  required owner: uuid;
  required title: str;

  policy owner_only for all to app_user
    using ( .owner = global current_user )
    with check ( .owner = global current_user );
}
```

lowers to (see [Globals](/asl/globals) for how `global current_user` becomes a
session read):

```sql
CREATE POLICY "owner_only" ON "doc" FOR ALL TO app_user
  USING (owner = current_setting('app.current_user', true)::UUID)
  WITH CHECK (owner = current_setting('app.current_user', true)::UUID);
```

## Traversing links

A predicate can follow links, not just read the policy's own columns.

**To-one chains** — `.organization.owner`, `.organization.owner.email` — lower to a
correlated subquery over the linked table:

```asl
type User { required email: str; }
type Organization { link owner: User; }

type Workflow {
  required name: str;
  link organization: Organization;

  policy owner_only for all to app_user
    using ( .organization.owner = global current_user );
}
```

lowers the `USING` clause to:

```sql
(SELECT o.owner FROM "organization" o WHERE o.id = "workflow".organization LIMIT 1)
  = current_setting('app.current_user', true)::UUID
```

**Membership** — `<value> in .<multi-link>` — tests whether a value is among the rows
reached through a multi-link, lowered to an `IN (SELECT …)` over the junction table:

```asl
type User { required email: str; }
type Organization {
  required name: str;
  multi members: User;

  policy member_can_read for select to app_user
    using ( global current_user in .members );
}
```

lowers the `USING` clause to:

```sql
current_setting('app.current_user', true)::UUID IN (
  SELECT u.id FROM "organization_members" jt JOIN "user" u ON u.id = jt.user
  WHERE jt.organization = "organization".id
)
```

A multi-link can only appear as the right side of `in` (it's a set, not a value);
using one in a scalar path — `.members.email = …` — is an error.

One limit remains:

- **No bind parameters.** A policy can't take a `$param`; pull request-scoped
  values in through a [`global`](/asl/globals) instead.

Policies are inherited from abstract parents, so a soft-delete guard can live on a
base type:

```asl
abstract type Soft {
  deleted_at: datetime;
  policy not_deleted for select using ( .deleted_at is null );
}
type Note extending Soft { required body: str; }
```

## The table owner bypasses RLS

`ENABLE ROW LEVEL SECURITY` applies to ordinary roles, but the **table owner (and
superusers) bypass it by default**. If your application connects as a non-owner
role (the recommended setup), policies apply as written. If it connects as the
owner, the policies are silently ignored — you'd need `FORCE ROW LEVEL SECURITY`,
which axel does not emit today.

This is usually what you want for a TTL/GC pattern: reads from the app role see
only live rows, while a privileged cleanup job (e.g. a [pg_cron](/asl/extensions)
sweep) still sees expired rows to delete them.

## Pairing with a one-time setup: `@for`

The cleanup job above is registered once with the `@for <Type>` function
directive — a function that axel **invokes a single time** in the migration that
first creates it (after the type's table exists), and tags to that type for
tracking:

```asl
use extension 'pg_cron';

@for KV
function kv_gc() -> int64 {
  return cron.schedule('kv-gc', '0 * * * *', 'DELETE FROM kv WHERE expires_at < now()');
};
```

The swept SQL can also be written as AQL and compiled in place, so the job is
checked against the schema — see [Inline AQL](/asl/functions#inline-aql-aql):

```asl
@for KV
function kv_gc() -> int64 {
  return cron.schedule('kv-gc', '0 * * * *', aql`delete KV filter .expires_at < now()`);
};
```

emitting, in that migration:

```sql
CREATE OR REPLACE FUNCTION "kv_gc"() RETURNS BIGINT AS $$ … $$ LANGUAGE plpgsql;
SELECT "kv_gc"();
```

Migrations are diffed by name and run in dependency order (extensions → tables →
functions → policies), so the extension exists before the function and the table
exists before its policy.
