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
  required updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();   # keep it fresh on every UPDATE
  };
}
```

> `default` only fires on INSERT, so without the `rewrite` line `updated_at` would
> never change. See [Rewrites](#rewrites).

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

Enums are stored as `TEXT` with a named `CHECK (col IN (...))` constraint (`chk_<table>_<col>_enum`)
restricting the column to the declared values.

```asl
enum Role { Admin, Member, Guest }
```

Use an enum as a property type. Reference an enum value in a default with the qualified
`Enum.Member` form (a quoted literal `'Member'` is also accepted); the value is validated against the
enum's declaration at `generate` time:

```asl
type User {
  required role: Role { default := Role.Member; };
}
```

This emits `"role" TEXT NOT NULL DEFAULT 'Member' CONSTRAINT "chk_user_role_enum" CHECK ("role" IN ('Admin', 'Member', 'Guest'))`.
Generated Go/TS code uses the enum type for the field (`Role`) rather than a plain string — for
model structs, query **parameters**, and query **result columns** alike (including columns pulled in
by a `*` splat and inside nested sub-select rows).

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

### Rewrites

A `default` runs once, on INSERT. A `rewrite` re-assigns the field on the events
you name — the mechanism behind an auto-updating `updated_at`:

```asl
updated_at: datetime {
  default := datetime_current();
  rewrite update := datetime_current();   # events: insert, update (comma-separated)
};
```

The value may be a builtin function (`datetime_current()` → `now()`), a literal,
or a row-reference column — `__new__.<field>` / `__subject__.<field>` (the row
being written) or `__old__.<field>` (the pre-update row, `UPDATE` only):

```asl
slug: str { rewrite create, update := __new__.title; };   # NEW."title"
```

Events are `insert` / `update`; `create` is accepted as an alias for `insert`.

A rewrite belongs to the type that **declared** it, and generates one function
per declaring model, named `axel_rw_<model>_<serial>`. An `updated_at` rewrite on
an abstract `Base` becomes a single `axel_rw_base_1` that **every** concrete type
inheriting it shares — each concrete table gets its own `BEFORE` trigger that
`EXECUTE`s that one function. A rewrite a concrete type declares itself is a
separate function (`axel_rw_<that_type>_1`), so a type that both inherits and
declares rewrites simply gets one trigger per contributing model. See
[Triggers](#triggers) for the general mechanism.

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

`min_length(n)` and `max_length(n)` apply to string columns and are emitted as a named
`CHECK (char_length("col") >= n)` / `<= n`. All field-level constraints carry a deterministic name
— enum and length `CHECK`s (`chk_<table>_<column>_<kind>`), single-column `UNIQUE`
(`uq_<table>_<column>`), primary keys (`pk_<table>`), and foreign keys (`fk_<table>_<column>`).
That same name is used both inside `CREATE TABLE` and in any later `ALTER TABLE ... ADD/DROP CONSTRAINT`,
so a constraint created with a table can be dropped by name on a schema change or rollback.

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

## Type-level constraints

In addition to field-level constraints, a `constraint <expr> on (.a, .b);` declaration inside a type
body applies a constraint across one or more columns. This is how you express composite constraints
such as unique-together.

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

---

## Functions

A top-level `function` declares a Postgres function. The body is **AQL by
default** (`body := ( … )`), or raw Postgres via dollar-quoting (`body := $$ … $$`).
A `-> trigger` function takes no parameters (Postgres rule) and is what a
[trigger](#triggers) executes.

```asl
# AQL body — compiled to plpgsql
function log_membership_changes() -> trigger {
  body := (
    insert AuditLog {
      table_name := 'organization',
      action := event,               # event = which op fired (INSERT/UPDATE/DELETE)
      new_data := to_jsonb(__new__)  # __new__ / __old__ = the changed row
    }
  );
};

# Raw Postgres body — the escape hatch
function slugify_name() -> trigger {
  language := plpgsql;               # default plpgsql; `sql` also valid for raw bodies
  body := $$
    BEGIN NEW.slug := lower(NEW.name); RETURN NEW; END;
  $$;
};
```

Inside an AQL body these **magic identifiers** are available:

| identifier | meaning | compiles to |
|---|---|---|
| `__new__` / `__old__` | the new / old row | `NEW` / `OLD` |
| `__new__.field` / `__old__.field` | a column of that row (validated against the enclosing type in an inline trigger `do`) | `NEW."col"` / `OLD."col"` |
| `__subject__` | the current row (also usable in `rewrite`) | `NEW` |
| `event` | which operation fired | `TG_OP` |

Everything else — bare identifiers and function calls like `TG_TABLE_NAME` or
`to_jsonb(__new__)` — passes through to SQL verbatim. Functions are emitted as
`CREATE OR REPLACE FUNCTION`; editing a body produces a single replace in the
migration.

## Triggers

A `trigger` inside a type body attaches to that table. Its body is either an
inline AQL statement (`do ( … )`, the default) or a reference to a declared
function (`execute <fn>()`).

```asl
type Application extends Base {
  required name: str;

  # Inline AQL — __new__.field is validated against Application
  trigger audit after insert, update, delete do (
    insert AuditLog {
      table_name := 'application',
      action := event,
      new_data := to_jsonb(__new__)
    }
  );

  # Or reference a declared function
  trigger touch before update execute slugify_name();
}
```

Timing is `before` | `after`; events are a comma-separated list of
`insert` / `update` / `delete`. Optional clauses: `for each row` (default) or
`for each statement`, and `when ( $$ <sql condition> $$ )`. An inline `do` body
compiles to a generated function plus the trigger; the SQL is ordered
extensions → tables → functions → triggers so a body that writes to another
table sees it exist.

> On a `DELETE`, `NEW` is null — don't reference `__new__` in a delete-only path.

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
