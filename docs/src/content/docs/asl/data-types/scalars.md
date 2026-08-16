---
title: "Scalars — ASL"
description: "Built-in scalar types and their PostgreSQL mappings"
---

# Built-in scalars

| ASL type   | PostgreSQL type    |
|------------|--------------------|
| `str`      | `TEXT`             |
| `int16`    | `SMALLINT`         |
| `int32`    | `INTEGER`          |
| `int64`    | `BIGINT`           |
| `float32`  | `REAL`             |
| `float64`  | `DOUBLE PRECISION` |
| `bool`     | `BOOLEAN`          |
| `uuid`     | `UUID`             |
| `datetime` | `TIMESTAMPTZ`      |
| `date`     | `DATE`             |
| `time`     | `TIME`             |
| `json`     | `JSONB`            |
| `bytes`    | `BYTEA`            |
| `decimal`  | `NUMERIC`          |

## Using `uuid` with `pgcrypto`

To generate UUIDs automatically (e.g. `default := gen_random_uuid();` or `gen_uuid()`), enable the `pgcrypto` PostgreSQL extension in your schema:

```asl
use extension 'pgcrypto';

type User {
  required id: uuid {
    default := gen_random_uuid();
    constraint pk;
  };
  required email: str;
}
```
