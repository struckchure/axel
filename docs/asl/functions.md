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

## Inline AQL: <code>aql`…`</code>

Some Postgres functions take **SQL as a string** — `cron.schedule`, `EXECUTE`,
`dblink`. Writing that SQL by hand means hand-maintaining table and column names
that your schema already knows. An <code>aql`…`</code> literal lets you write
[AQL](/aql/) instead: axel compiles it while generating the migration and inlines
the result as a quoted SQL string.

Both forms below are valid, and emit the same migration:

```asl
@for KV
function kv_gc() -> int64 {
  return cron.schedule('kv-gc', '0 * * * *', 'DELETE FROM "kv" WHERE expires_at < now()');
};

@for KV
function kv_gc() -> int64 {
  return cron.schedule('kv-gc', '0 * * * *', aql`delete KV filter .expires_at < now()`);
};
```

```sql
CREATE OR REPLACE FUNCTION "kv_gc"() RETURNS BIGINT AS $$
BEGIN
  RETURN cron.schedule('kv-gc', '0 * * * *', 'DELETE FROM "kv" k WHERE k.expires_at < now();');
END;
$$ LANGUAGE plpgsql;
```

The literal is a **compile-time** construct — nothing about it survives into the
database. What you get for it:

- The query is checked against your schema. A renamed property or a deleted type
  fails `axel diff` (and is underlined in the editor) instead of failing at 3am
  inside a cron job.
- Table and column names come from the schema, so `snake_case` derivation and
  quoting are handled the same way `CREATE TABLE` handles them.

Two rules:

- **No query parameters.** The compiled SQL is embedded as a literal, so there is
  nothing to bind `$name` to — a parameterized inline query is an error. Take a
  function parameter and concatenate if you need a value.
- **Backticks are the delimiter**, and the `aql` tag is required — a bare
  `` `…` `` is a parse error.

An inline query is a complete AQL statement, so any of `select` / `insert` /
`update` / `delete` works. The trailing `;` is optional.

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
| `@for <Type>` | nothing directly — marks a **run-once** setup function, invoked once (`SELECT fn();`) in the migration that first creates it, and tags it to `<Type>`. See [Policies](/asl/policies#pairing-with-a-one-time-setup-for). |

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
