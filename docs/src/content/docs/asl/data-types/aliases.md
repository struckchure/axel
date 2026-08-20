---
title: "Aliases & Typed JSON — ASL"
description: "Named scalar aliases and typed JSON scalar definitions"
---

# Named scalar aliases

Create a named alias over a built-in [scalar](/asl/data-types/scalars) using `extends`.

```asl
scalar type EmailStr extends str;
scalar type Score extends float32;
```

> **Deprecation Notice:** The `extending` keyword is deprecated in favor of `extends`. Running `axel fmt` will automatically migrate `extending` to `extends`.

Use aliases like any other type:

```asl
type User {
  required email: EmailStr;
}
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

