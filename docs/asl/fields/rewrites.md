---
title: Rewrites — ASL
description: Auto-assign a field on insert/update events
---

# Rewrites

A [`default`](/asl/fields/properties#defaults) runs once, on INSERT. A `rewrite` re-assigns
the field on the events you name — the mechanism behind an auto-updating
`updated_at`:

```asl
updated_at: datetime {
  default := datetime_current();
  rewrite update := datetime_current();   # events: insert, update (comma-separated)
};
```

The value may be a builtin function (`datetime_current()` → `now()`), a literal,
or a row-reference column — `__new__.<field>` / `__subject__.<field>` (the row
being written) or `__old__.<field>` (the pre-update row, `UPDATE` only):

```asl
slug: str { rewrite create, update := __new__.title; };   # NEW."title"
```

Events are `insert` / `update`; `create` is accepted as an alias for `insert`.

A rewrite belongs to the type that **declared** it, and generates one function
per declaring model, named `axel_rw_<model>_<serial>`. An `updated_at` rewrite on
an abstract `Base` becomes a single `axel_rw_base_1` that **every** concrete type
inheriting it shares — each concrete table gets its own `BEFORE` trigger that
`EXECUTE`s that one function. A rewrite a concrete type declares itself is a
separate function (`axel_rw_<that_type>_1`), so a type that both inherits and
declares rewrites simply gets one trigger per contributing model. See
[Triggers](/asl/triggers) for the general mechanism.
