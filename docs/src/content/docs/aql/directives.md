---
title: "Directives — AQL"
description: "Codegen metadata with @name, @request, and @response"
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
| `@rel_load_strategy <join\|query>` | Sets the relation loading strategy (`query` or `join`) for this query |

### Formatting Directives

When formatting AQL files (via `axel fmt` or language server formatters), a blank line is automatically placed after the directive declarations block before the query statement.

### Relation Loading Strategies

By default, nested relations and shapes are compiled using correlated subqueries in the `SELECT` projection (`query` strategy). You can override this globally in `axel.yaml` via `rel-load-strategy` or per-query with `@rel_load_strategy`:

- **`query` (default)**: Uses correlated subqueries with `json_agg` (multi-links) and `row_to_json` (single-links) in the `SELECT` projection.
- **`join`**: Uses `LEFT JOIN LATERAL` subqueries in the SQL `FROM` clause.

```aql
@name GetUserWithPosts
@rel_load_strategy join

select User {
  id,
  email,
  posts: { id, title }
} filter .id = $id;
```

Unknown directives are parsed and preserved (and exposed to external generators) but otherwise
ignored. A `@response`/`@request` name may be **shared** across queries: the type is generated
once and reused. If two queries give the same name but different fields — or a name collides
with an existing schema type of a different shape — code generation **aborts** with a conflict
error. (`@name` replaces the older `# @name` comment, which is no longer recognized.)
