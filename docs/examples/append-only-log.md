---
title: Append-only event log — Examples
description: Block updates and deletes with a single multi-command policy
---

# Append-only event log

An audit or event table should only ever grow: rows are inserted, never changed
or removed. One [policy](/asl/policies) with a multi-command `for` clause locks
both `UPDATE` and `DELETE` for the application role.

```asl
enum EventKind { Created, Updated, Deleted }

type Event {
  required topic: str;
  required kind: EventKind;
  payload: json;
  required actor: str;
  required at: datetime { default := now() };

  # A DELETE has no "new row" to check, so block both writes with `using (false)`
  # — no existing row is ever visible to UPDATE or DELETE.
  policy append_only for update, delete using ( false );
}
```

Because Postgres allows one command per `CREATE POLICY`, the `for update, delete`
list expands to two policies (suffixed to keep their names unique):

```sql
ALTER TABLE "event" ENABLE ROW LEVEL SECURITY;
CREATE POLICY "append_only_update" ON "event" FOR UPDATE USING (false);
CREATE POLICY "append_only_delete" ON "event" FOR DELETE USING (false);
```

Inserts still work; updates and deletes from the app role affect zero rows.

```aql
insert Event { topic := $topic, kind := EventKind.Created, actor := $actor };
```

::: warning `with check` vs `using`
It's tempting to write `for update, delete with check ( false )`, but Postgres
rejects `WITH CHECK` on `DELETE` (there's no new row to validate). Axel catches
this at `axel validate` / `axel diff` — before it hits the database — and points
you at `using ( false )`, which blocks both by making existing rows invisible to
the write. See [Policies → which clause each command accepts](/asl/policies#syntax).
:::

::: tip The table owner bypasses RLS
As with any policy, a privileged connection (the table owner or a superuser)
bypasses RLS, so a maintenance job can still prune old events. Connect your
application as a **non-owner role** for the guard to apply. See
[Policies → the table owner bypasses RLS](/asl/policies). This is also why hosted
Postgres providers like [Supabase](/integrations/supabase) lean on RLS: the app
connects as a restricted role while the service role bypasses it.
:::
