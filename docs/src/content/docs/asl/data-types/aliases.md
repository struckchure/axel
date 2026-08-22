---
title: "Aliases & Typed JSON — ASL"
description: "Named scalar aliases and typed JSON scalar definitions"
---

# Named scalar aliases & Extended Types

Create a named alias or extended scalar type over a built-in [scalar](/asl/data-types/scalars) or another scalar type using `extends`.

```asl
scalar type EmailStr extends str;
scalar type Score extends float32;
```

> **Deprecation Notice:** The `extending` keyword is deprecated in favor of `extends`. Running `axel fmt` will automatically migrate `extending` to `extends`.

### Extended Scalars with Field Descriptors

Scalar types can define field descriptors (`constraint`, `default`, `rewrite`) inside their body block `{ ... }`, just like properties in object type models.

```asl
scalar type Code extends str {
  constraint min_length(6);
  constraint max_length(6);
  default := random_hex(6);
}

scalar type AutoTimestamp extends datetime {
  rewrite update := datetime_current();
}
```

Any object type property declared with an extended scalar automatically inherits all of its field descriptors:

```asl
type Product {
  required id: uuid { constraint pk; };
  # Inherits min_length(6), max_length(6), and default random_hex(6):
  code: Code;
  # Properties can override the default and add additional constraints:
  custom_code: Code {
    default := '999999';
    constraint exclusive;
  };
  # Inherits the BEFORE UPDATE trigger rewrite:
  updated_at: AutoTimestamp;
}
```

### Chained Scalar Inheritance

Scalar types can extend other user-defined scalar types, inheriting constraints, defaults, and rewrites in a chain:

```asl
scalar type ShortStr extends str {
  constraint max_length(10);
}

scalar type ExactCode extends ShortStr {
  constraint min_length(6);
  default := '000000';
}
```

---

# Custom SQL Extension Scalars

For PostgreSQL extension types (such as PostGIS `geography`, `geometry`, `pgvector` `vector`, `citext`, `ltree`), declare custom SQL scalars with `extends sql "<type>"`. You can optionally supply client-side representation typing using `as`:

```asl
# Record representation: generates interfaces/structs in codegen, enables AQL dot-access (.loc.lat)
# __self__ refers to the column instance in AQL queries
scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32 := ST_Y(__self__::geometry);
  longitude: float32 := ST_X(__self__::geometry);
};

# Multi-dimensional array representation: generates number[] / []float32 in codegen
scalar type Embedding extends sql "vector(1536)" as multi float32;

# Primitive scalar mapping: generates string in codegen
scalar type Citext extends sql "citext" as str;

# Opaque type: defaults to string in codegen
scalar type Geometry extends sql "geometry";
```

---

# Typed JSON Scalars

You can define structured, typed JSON and JSONB scalars that give type safety, autocomplete, and query capabilities to document data in PostgreSQL.

```asl
scalar type Coordinate extends json {
  lat: str;
  lng: str;
}

scalar type ItemStats extends jsonb {
  score: float64;
  views: int32;
  multi tags: str;
}
```

### Allowed Field Types

To ensure reliable extraction and type coercion in PostgreSQL, field types within typed JSON scalars are strictly restricted to:
- **Strings**: `str`
- **Numbers**: `int16`, `int32`, `int64`, `float32`, `float64`, `decimal`
- **Arrays**: Provided via the `multi` modifier (e.g. `multi tags: str;`, `multi scores: float64;`)

Nested JSON objects and non-primitive types are not permitted within typed JSON scalars.

### Using Typed JSON Scalars in Types

Declare properties with your typed JSON scalar:

```asl
type Place {
  required id: uuid;
  name: str;
  coord: Coordinate;
  stats: ItemStats;
}
```

### Querying in AQL

You can query and filter on individual fields of a typed JSON scalar directly using dot-notation:

```aql
# String comparison automatically extracts as text (coord->>'lat')
select Place { id, name } filter .coord.lat = $lat;

# Numeric comparisons automatically apply PostgreSQL type casting ((stats->>'score')::DOUBLE PRECISION)
select Place { id, name } filter .stats.score > $min_score;

# Integer comparisons
select Place { id, name } filter .stats.views >= 100;
```

### Generated Client Types

Axel generates idiomatic types for all target languages:

#### TypeScript
```typescript
export interface Coordinate {
  lat?: string | null;
  lng?: string | null;
}

export interface ItemStats {
  score?: number | null;
  views?: number | null;
  tags?: string[] | null;
}

export interface Place {
  id: string;
  name?: string | null;
  coord?: Coordinate | null;
  stats?: ItemStats | null;
}
```

#### Go
```go
type Coordinate struct {
	Lat *string `json:"lat"`
	Lng *string `json:"lng"`
}

type ItemStats struct {
	Score *float64 `json:"score"`
	Views *int32   `json:"views"`
	Tags  []string `json:"tags"`
}

type Place struct {
	ID    string      `json:"id" db:"id"`
	Name  *string     `json:"name" db:"name"`
	Coord *Coordinate `json:"coord" db:"coord"`
	Stats *ItemStats  `json:"stats" db:"stats"`
}
```

