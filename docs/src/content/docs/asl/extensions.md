---
title: "Extensions — ASL"
description: "Enable Postgres extensions with `use extension`"
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
migration, and an unchanged declaration produces no diff.

Nothing is enabled automatically. In particular, `gen_uuid()` defaults compile to
`gen_random_uuid()`, which requires `pgcrypto` — so declare it explicitly:

```asl
use extension 'pgcrypto';
```

A common pairing is an extension plus a function that uses it:

```asl
use extension 'unaccent';

@language plpgsql
@immutable
function slugify(value: text) -> text {
  return lower(public.unaccent(value));
};
```

## Custom Extension Types

Extensions like **PostGIS** (`postgis`), **pgvector** (`vector`), and **citext** introduce custom PostgreSQL data types. You can declare scalars that map directly to these extension types using `extends sql "<type>"`, and define client-side typing with `as`:

```asl
use extension 'postgis';
use extension 'vector';
use extension 'citext';

# 1. Structured record representation (for codegen interfaces & AQL dot-access)
scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32;
  longitude: float32;
};

# 2. Multi-dimensional array representation (codegen emits number[] / []float32)
scalar type Embedding extends sql "vector(1536)" as multi float32;

# 3. Primitive scalar alias (codegen emits string)
scalar type Citext extends sql "citext" as str;

# 4. Opaque custom SQL type (defaults to 'str' in host codegen)
scalar type Geometry extends sql "geometry";

type Venue {
  required id: uuid { constraint pk; };
  name: Citext;
  location: Point;
  feature_vec: Embedding;
  geom: Geometry;
}
```

### Benefits:
- **Exact DDL**: Generated migration SQL produces columns with the precise PostgreSQL type (e.g. `geography(Point, 4326)`, `vector(1536)`).
- **Client Typing**: SDK generators produce typed interfaces/structs (`Point` as `{ latitude: number, longitude: number }`, `Embedding` as `number[]`).
- **AQL Dot-Access**: Structured fields can be traversed directly in AQL expressions (`.location.latitude`).

See [Functions](/asl/functions) for the full function syntax and [Aliases & Extended Types](/asl/data-types/aliases) for more on custom scalar types.
