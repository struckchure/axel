---
title: Triggers — ASL
description: Row and statement triggers attached to a type
---

# Triggers

A `trigger` inside a type body attaches to that table. Its body is either an
inline AQL statement (`do ( … )`, the default) or a reference to a declared
[function](/asl/functions) (`execute <fn>()`).

```asl
type Application extends Base {
  required name: str;

  # Inline AQL — __new__.field is validated against Application
  trigger audit after insert, update, delete do (
    insert AuditLog {
      table_name := 'application',
      action := event,
      new_data := to_jsonb(__new__)
    }
  );

  # Or reference a declared function
  trigger touch before update execute slugify_name();
}
```

Timing is `before` | `after`; events are a comma-separated list of
`insert` / `update` / `delete`. Optional clauses: `for each row` (default) or
`for each statement`, and `when ( $$ <sql condition> $$ )`. An inline `do` body
compiles to a generated function plus the trigger; the SQL is ordered
extensions → tables → functions → triggers so a body that writes to another
table sees it exist.

> On a `DELETE`, `NEW` is null — don't reference `__new__` in a delete-only path.

See [Functions](/asl/functions) for the magic identifiers (`__new__`, `event`, …)
available inside an inline `do` body.
