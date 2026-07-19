---
title: Constraints — ASL
description: Field-level exclusive, pk, and CHECK constraints
---

# Field constraints

```asl
email: str {
  constraint exclusive;          # UNIQUE
  constraint min_length(5);
  constraint max_length(100);
};

id: uuid {
  constraint pk;                 # PRIMARY KEY
  constraint exclusive;          # UNIQUE
};
```

`min_length(n)` and `max_length(n)` apply to string columns and are emitted as a named
`CHECK (char_length("col") >= n)` / `<= n`. All field-level constraints carry a deterministic name
— enum and length `CHECK`s (`chk_<table>_<column>_<kind>`), single-column `UNIQUE`
(`uq_<table>_<column>`), primary keys (`pk_<table>`), and foreign keys (`fk_<table>_<column>`).
That same name is used both inside `CREATE TABLE` and in any later `ALTER TABLE ... ADD/DROP CONSTRAINT`,
so a constraint created with a table can be dropped by name on a schema change or rollback.

For constraints that span multiple columns, see
[type-level constraints](/asl/schema/constraints).
