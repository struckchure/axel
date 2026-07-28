---
title: Extensions — ASL
description: Enable Postgres extensions with `use extension`
---

# Extensions

`use extension '<name>';` enables a Postgres extension. It lowers to
`CREATE EXTENSION IF NOT EXISTS`, emitted **before** tables and functions in a
migration (and dropped last on the way down), so anything that depends on it —
a function, a default, a column type — is created after it exists.

```asl
use extension 'unaccent';
use extension 'uuid-ossp';   # quoted, so hyphenated names work
```

```sql
CREATE EXTENSION IF NOT EXISTS "unaccent";
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
```

Extensions are tracked in the migration snapshot: declaring one adds a
`CREATE EXTENSION`, removing the declaration adds a `DROP EXTENSION` to the next
migration, and an unchanged declaration produces no diff. (`pgcrypto` is always
enabled automatically for `gen_uuid()` defaults — you don't declare it.)

A common pairing is an extension plus a function that uses it:

```asl
use extension 'unaccent';

@language plpgsql
@immutable
function slugify(value: text) -> text {
  return lower(public.unaccent(value));
};
```

See [Functions](/asl/functions) for the full function syntax.
