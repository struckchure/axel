---
title: Schema Language (ASL)
description: Define PostgreSQL schemas with Axel Schema Language
---

# Axel Schema Language (ASL)

ASL is a declarative schema language for defining PostgreSQL types. You write `.asl` files; Axel compiles them into migration SQL that you apply with `axel generate` and `axel up`.

---

## File extension

```
schema.asl
```

---

## Types

### Concrete types

A concrete type maps to a database table.

```asl
type User {
  required email: str;
  name: str;
  required age: int32;
}
```

The keyword `model` is accepted as a synonym for `type`.

### Abstract types

Abstract types have no table of their own. They exist only to be extended by other types.

```asl
abstract type Timestamped {
  required id: uuid {
    default := gen_uuid();
    constraint pk;
  };
  required created_at: datetime { default := datetime_current(); };
  required updated_at: datetime { default := datetime_current(); };
}
```

### Extending types

A type can extend one or more other types. All properties, links, indexes, and computed fields are inherited.

```asl
type User extending Timestamped {
  required email: str;
}

type Admin extending User, Audited {
  required level: int32;
}
```

The keyword `extends` is accepted as a synonym for `extending`.

---

## Scalar types

### Built-in scalars

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

### Named scalar aliases

Create a named alias over a built-in scalar.

```asl
scalar type EmailStr extending str;
scalar type Score extending float32;
```

Use aliases like any other type:

```asl
type User {
  required email: EmailStr;
}
```

---

## Enum types

Enums are stored as `TEXT` with a `CHECK` constraint.

```asl
enum Role { Admin, Member, Guest }
```

Use an enum as a property type:

```asl
type User {
  required role: Role;
}
```

---

## Properties

Properties map to columns.

```asl
type User {
  required email: str;           # NOT NULL column
  name: str;                     # nullable column
  required property age: int32;  # "property" keyword is optional
}
```

### Required

`required` maps to `NOT NULL`.

### Defaults

```asl
active: bool { default := true };
name: str    { default := 'anonymous'; };
score: int32 { default := 0; };

# Functions
id:         uuid     { default := gen_uuid(); };
created_at: datetime { default := datetime_current(); };
```

### Constraints

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

---

## Links

Links define foreign-key relationships between types.

### Single link (FK column)

```asl
type Post {
  required link author: User;    # adds author_id FK column
}
```

### Multi link (junction table)

```asl
type Post {
  multi link tags: Tag;          # creates post_tags junction table
}
```

The junction table name is `{source}_{link}` in snake_case (e.g. `post_tags`).

### Required links

```asl
type Comment {
  required link post: Post;      # post_id NOT NULL
  required link author: User;    # author_id NOT NULL
}
```

---

## Computed fields

Computed fields are not stored as columns. They are expanded inline during AQL compilation.

```asl
type User {
  required email: str;
  name: str;
  computed display_name := .name ?? .email;
}
```

Use `??` for a null-coalescing fallback. Computed fields can be referenced in AQL shapes.

---

## Indexes

```asl
type User {
  required email: str;
  required age: int32;
  active: bool;

  index on (.email);
  index on (.active, .age);
}
```

Each `index on (...)` declaration generates a `CREATE INDEX` statement in the migration SQL.

---

## Complete example

```asl
scalar type EmailStr extending str;

enum Role { Admin, Member, Guest }

abstract type Base {
  required id: uuid {
    default := gen_uuid();
    constraint pk;
  };
  required created_at: datetime { default := datetime_current(); };
  required updated_at: datetime { default := datetime_current(); };
}

type User extending Base {
  required email: EmailStr {
    constraint exclusive;
  };
  name: str;
  required age: int32;
  active: bool { default := true };
  required role: Role;

  computed display_name := .name ?? .email;

  index on (.email);
}

type Post extending Base {
  required title: str;
  required content: str;
  required link author: User;
  multi link likes: User;
}

type Comment extending Base {
  required link post: Post;
  required link author: User;
  required content: str;
}
```
