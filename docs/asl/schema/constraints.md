---
title: Constraints — ASL
description: Composite, type-level constraints spanning multiple columns
---

# Type-level constraints

In addition to [field-level constraints](/asl/fields/constraints), a `constraint <expr> on (.a, .b);`
declaration inside a type body applies a constraint across one or more columns. This is how you
express composite constraints such as unique-together.

```asl
type Membership {
  required user_id: uuid;
  required org_id: uuid;
  required code: str;

  constraint exclusive on (.user_id, .org_id);   # composite UNIQUE (unique together)
  constraint min_length(4) on (.code);           # CHECK on char_length
}
```

Supported expressions: `exclusive` → composite `UNIQUE`, `pk` → composite `PRIMARY KEY`,
`min_length(n)` / `max_length(n)` → `char_length` `CHECK`. Constraints are emitted with deterministic
names (e.g. `uq_membership_user_id_org_id`) inside `CREATE TABLE`, and adding or removing one on an
existing type generates an `ALTER TABLE ... ADD/DROP CONSTRAINT` in the migration SQL.
