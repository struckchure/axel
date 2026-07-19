---
title: Schema Language (ASL)
description: Define PostgreSQL schemas with Axel Schema Language
---

# Axel Schema Language (ASL)

ASL is a declarative schema language for defining PostgreSQL types. You write `.asl` files; Axel compiles them into migration SQL that you apply with `axel generate` and `axel up`.

```
schema.asl
```

## How ASL is organized

An ASL file is a set of top-level declarations. The reference is split by feature:

- **[Schema](/asl/schema)** — concrete and abstract types, inheritance (`extending`), indexes, and composite constraints.
- **[Data Types](/asl/data-types)** — built-in scalars, named scalar aliases, and enums.
- **[Fields](/asl/fields)** — properties, defaults, rewrites, field constraints, links, and computed fields.
- **[Functions](/asl/functions)** — top-level Postgres functions with AQL or raw-SQL bodies.
- **[Triggers](/asl/triggers)** — row/statement triggers attached to a type.

## Complete example

A schema that touches most features:

```asl
scalar type EmailStr extending str;

enum Role { Admin, Member, Guest }

abstract type Base {
  required id: uuid {
    default := gen_uuid();
    constraint pk;
  };
  required created_at: datetime { default := datetime_current(); };
  required updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();
  };
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
