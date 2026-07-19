---
title: Directives — AQL
description: Codegen metadata with @name, @request, and @response
---

# Directives

A query file may begin with `@<directive> <value>` declarations. Directives are real AQL syntax
(parsed into the AST), not comments, and they carry code-generation metadata:

```aql
@name CreateUser
@request CreateUserInput
@response User

insert User { email := $email, name := $name };
```

| Directive | Effect |
|-----------|--------|
| `@name <Name>` | Sets the generated query/function name (default: derived from the filename) |
| `@request <Name>` | Names the generated params type (default: `<Query>Params`) |
| `@response <Name>` | Names the generated row type (default: `<Query>Row`) |

Unknown directives are parsed and preserved (and exposed to external generators) but otherwise
ignored. A `@response`/`@request` name may be **shared** across queries: the type is generated
once and reused. If two queries give the same name but different fields — or a name collides
with an existing schema type of a different shape — code generation **aborts** with a conflict
error. (`@name` replaces the older `# @name` comment, which is no longer recognized.)
