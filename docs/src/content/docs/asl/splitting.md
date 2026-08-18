---
title: "Splitting a Schema — ASL"
description: "Spread a schema across several .asl files with a glob"
---

# Splitting a Schema

A schema does not have to live in one file. Point `schema-path` at a glob, and
Axel reads every matching `.asl` file and merges them into one schema:

```yaml
# axel.yaml
schema-path: schema/*.asl
```

```
schema/
  base.asl      # abstract types shared by everything
  user.asl
  post.asl
  functions.asl
```

Every command that takes a schema accepts the same forms:

| Form               | Meaning                                          |
|--------------------|--------------------------------------------------|
| `schema.asl`       | a single file                                     |
| `schema/`          | a directory, walked recursively for `.asl` files  |
| `schema/*.asl`     | every `.asl` directly inside `schema/`            |
| `schema/**/*.asl`  | every `.asl` under `schema/`, at any depth        |

```sh
axel validate --schema 'schema/*.asl'
axel codegen --schema-path 'schema/**/*.asl' -g go -o ./gen
```

Quote the pattern so Axel expands it rather than your shell — that way `**`
works the same everywhere.

If you run `axel -d ./myproject` with no `axel.yaml`, a `schema/` directory in
the project is picked up automatically once `schema.asl` and `default.asl` are
absent.

## One flat namespace

There is no `import` and no per-file scoping. The files are concatenated, so a
declaration in one file is visible from all the others:

```asl
# schema/base.asl
abstract type Base {
  required id: uuid { default := gen_uuid(); constraint pk; };
}
```

```asl
# schema/post.asl
type Post extending Base {
  required title: str;
  required link author: User;   # User lives in schema/user.asl
}
```

Order never matters — not within a file, and not between files. A type may
extend a parent declared in a file that is read later, and a link may point at a
type declared anywhere in the set.

`use extension` is deduplicated, so each file is free to declare the extensions
it depends on:

```asl
use extension 'uuid-ossp';
```

## Duplicate names are an error

Because the files share one namespace, declaring the same name twice is
rejected, and the error names both locations:

```
resolving schema: type "User" declared more than once (schema/user.asl:3:1 and schema/billing.asl:14:1)
```

The same applies to enums, scalar aliases, globals and functions — and across
kinds, so an enum and a type cannot share a name either.
