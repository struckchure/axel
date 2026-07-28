---
title: Functions — ASL
description: Top-level Postgres functions with parameters, directives, and a return expression
---

# Functions

A top-level `function` declares a Postgres function. Axel emits it as
`CREATE OR REPLACE FUNCTION` — mapped **directly** to Postgres, not aliased, so it
is callable from queries, other functions, triggers, defaults, and checks.

```asl
use extension 'unaccent';

@language plpgsql
@immutable
@strict
@parallel safe
function slugify(value: text) -> text {
  return regexp_replace(
    regexp_replace(lower(public.unaccent(value)), '[^a-z0-9\-_]+', '-', 'gi'),
    '(^-+|-+$)', '', 'g'
  );
};
```

lowers to:

```sql
CREATE OR REPLACE FUNCTION "slugify"(value text) RETURNS text AS $$
BEGIN
  RETURN regexp_replace(
    regexp_replace(lower(public.unaccent(value)), '[^a-z0-9\-_]+', '-', 'gi'),
    '(^-+|-+$)', '', 'g'
  );
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT PARALLEL SAFE;
```

## Parameters and types

Parameters are `name: type`. Types map **directly to Postgres**: ASL scalars
(`str`, `int32`, …) and your own scalar aliases/enums map to their SQL type
(`str` → `TEXT`), and **any other name passes through verbatim** as a raw Postgres
type. A trailing `[]` makes an array.

```asl
@language sql
function first_tag(tags: text[]) -> text { return tags[1]; };   # raw text[] passthrough

@language sql
function total(a: int32, b: int32) -> int32 { return a + b; };  # ASL scalars → INTEGER
```

## The body

A function body is a single `return <expr>;`. The expression is **raw Postgres**,
passed through verbatim, and axel wraps it for you:

| `@language` | wrapper |
|---|---|
| `plpgsql` (default) | `BEGIN RETURN <expr>; END;` |
| `sql` | `SELECT <expr>;` |

```asl
@language sql
function full_name(first: text, last: text) -> text {
  return first || ' ' || last;
};
```

Everything in the expression — operators, function calls (`lower`, `regexp_replace`,
`public.unaccent`), casts, subqueries — passes straight through to Postgres, so the
full SQL surface is available. For multi-statement logic (mutations that also
return a row), use a [trigger](/asl/triggers) with an inline `do ( … )` body.

## Directives (attributes)

Attributes are declared as directives **above** the function, decorator-style.
The value is omitted for flags and given for the rest:

| Directive | Emits |
|---|---|
| `@language <lang>` | `LANGUAGE <lang>` (default `plpgsql`; `sql` for pure-expression functions) |
| `@immutable` / `@stable` / `@volatile` | the volatility class |
| `@strict` | `STRICT` (returns null on any null argument) |
| `@leakproof` | `LEAKPROOF` |
| `@parallel safe` / `unsafe` / `restricted` | `PARALLEL …` |
| `@security definer` / `invoker` | `SECURITY …` |
| `@cost <n>` | `COST <n>` |

Attributes are emitted in a fixed order, so re-ordering directives never produces
a spurious migration.

## Trigger functions

A `-> trigger` function takes no parameters (Postgres rule) and is what a
[trigger](/asl/triggers)'s `execute` form runs. Its `return` yields the row:

```asl
function stamp() -> trigger { return NEW; };

type Post {
  id: uuid { default := gen_uuid(); };
  title: str;
  trigger t before insert execute stamp();
}
```

Functions are emitted as `CREATE OR REPLACE FUNCTION`; editing a definition
produces a single replace in the migration.
