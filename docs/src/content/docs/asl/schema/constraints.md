---
title: "Constraints — ASL"
description: "Composite, type-level constraints spanning multiple columns"
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

## Partial (filtered) unique constraints

An `exclusive` constraint can carry a `filter <predicate>` clause, making it a
**partial** unique constraint — the uniqueness only applies to rows matching the
predicate. The predicate is a native [AQL](/aql/) expression (the same language as
[policies](/asl/policies) and query filters), including `Enum.Member` references.

```asl
enum QueueStatus { Pending, Running, Done }

type Job {
  required name: str;
  required actor: str;
  required status: QueueStatus;

  # At most one Pending job per (name, actor); Running/Done rows are unconstrained.
  constraint exclusive on (.name, .actor) filter .status = QueueStatus.Pending;
}
```

Postgres can't put a `WHERE` on a table constraint, so a filtered `exclusive`
lowers to a **partial unique index** rather than a `CONSTRAINT … UNIQUE`:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS "uq_job_name_actor"
  ON "job" ("name", "actor")
  WHERE (status = 'Pending');
```

See the [job queue example](/examples/job-queue) for the full pattern.
