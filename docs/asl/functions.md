---
title: Functions — ASL
description: Top-level Postgres functions with AQL or raw-SQL bodies
---

# Functions

A top-level `function` declares a Postgres function. The body is **AQL by
default** (`body := ( … )`), or raw Postgres via dollar-quoting (`body := $$ … $$`).
A `-> trigger` function takes no parameters (Postgres rule) and is what a
[trigger](/asl/triggers) executes.

```asl
# AQL body — compiled to plpgsql
function log_membership_changes() -> trigger {
  body := (
    insert AuditLog {
      table_name := 'organization',
      action := event,               # event = which op fired (INSERT/UPDATE/DELETE)
      new_data := to_jsonb(__new__)  # __new__ / __old__ = the changed row
    }
  );
};

# Raw Postgres body — the escape hatch
function slugify_name() -> trigger {
  language := plpgsql;               # default plpgsql; `sql` also valid for raw bodies
  body := $$
    BEGIN NEW.slug := lower(NEW.name); RETURN NEW; END;
  $$;
};
```

## Magic identifiers

Inside an AQL body these **magic identifiers** are available:

| identifier | meaning | compiles to |
|---|---|---|
| `__new__` / `__old__` | the new / old row | `NEW` / `OLD` |
| `__new__.field` / `__old__.field` | a column of that row (validated against the enclosing type in an inline trigger `do`) | `NEW."col"` / `OLD."col"` |
| `__subject__` | the current row (also usable in `rewrite`) | `NEW` |
| `event` | which operation fired | `TG_OP` |

Everything else — bare identifiers and function calls like `TG_TABLE_NAME` or
`to_jsonb(__new__)` — passes through to SQL verbatim. Functions are emitted as
`CREATE OR REPLACE FUNCTION`; editing a body produces a single replace in the
migration.
