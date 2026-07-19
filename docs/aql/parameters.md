---
title: Parameters — AQL
description: Named, optional, and typed query parameters
---

# Parameters

Named parameters use a `$` prefix. They are collected in order of first appearance
and mapped to positional `$N` SQL parameters.

- **[Named](/aql/parameters/named)** — the basics of `$name` parameters.
- **[Optional](/aql/parameters/optional)** — `$name?` and how it behaves in filters vs `set`.
- **[Typed](/aql/parameters/typed)** — `$name<type>` annotations and type inference.
