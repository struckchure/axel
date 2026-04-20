---
title: Code Generation
description: Generate Go, TypeScript, or custom code from your schema and AQL queries
---

# Code Generation

Axel can generate type-safe code from your ASL schema and compiled AQL queries. Two generators are built in — Go and TypeScript — and you can write your own in any language.

---

## Quick start

```sh
# Go
axel -d ./myproject codegen -g go -o ./gen

# TypeScript
axel -d ./myproject codegen -g ts -o ./gen
```

Axel auto-discovers all `*.aql` files under the project directory and compiles them together with the schema.

---

## `axel codegen`

```
axel codegen [flags] [query-files...]
```

| Flag            | Short | Default | Description |
|-----------------|-------|---------|-------------|
| `--generator`   | `-g`  |         | Built-in generator name (`go` or `ts`) |
| `--plugin`      | `-p`  |         | Path to an external generator binary |
| `--out-dir`     | `-o`  | `.`     | Directory to write generated files into |
| `--query`       | `-q`  |         | AQL file or glob pattern (repeatable) |
| `--schema-path` |       |         | Schema file (default: from config or `axel/schema.asl`) |
| `--option`      |       |         | `key=value` passed to the generator (repeatable) |

`--generator` and `--plugin` are mutually exclusive.

### Query file discovery

Query files are resolved in this order:

1. `-q` patterns — Axel expands these (supports `**/*.aql`)
2. Positional arguments — shell-expanded paths
3. Auto-discovery — all `*.aql` files under `--dir` when nothing else is given

```sh
# Explicit list
axel codegen -g go -o ./gen -q 'queries/**/*.aql'

# Auto-discover from project dir
axel -d ./myproject codegen -g go -o ./gen

# Mix: all queries plus one extra
axel codegen -g go -o ./gen -q 'queries/*.aql' extra.aql
```

### Query name annotation

By default the function name is derived from the filename (`list_post.aql` → `listPost`). Override it with a `# @name` annotation on the first line:

```aql
# @name GetActiveUsers
select User { id, email } filter .active = true;
```

---

## TypeScript generator (`-g ts`)

### Generated files

| File | Contents |
|------|----------|
| `models.ts` | One `interface` per concrete ASL type; one `type` alias per enum |
| `<query_name>.ts` | Typed async function per AQL query with params and row interfaces |
| `runner.ts` | `Runner` class, `Queries` class, builder infrastructure, embedded schema |

### Type mapping

| AQL type      | TypeScript type  | Nullable TypeScript type |
|---------------|------------------|--------------------------|
| `str`         | `string`         | `string \| null`         |
| `int16/32/64` | `number`         | `number \| null`         |
| `float32/64`  | `number`         | `number \| null`         |
| `bool`        | `boolean`        | `boolean \| null`        |
| `uuid`        | `string`         | `string \| null`         |
| `datetime`    | `Date`           | `Date \| null`           |
| `json`        | `unknown`        | `unknown`                |

### Setup

The generator targets Bun's SQL client. The generated `DB` interface is:

```ts
export interface DB {
  unsafe<T = Record<string, unknown>>(sql: string, params?: unknown[]): Promise<T[]>;
}
```

Bun's `SQL` class satisfies this directly:

```ts
import { SQL } from "bun";
import { Runner } from "./gen/runner.ts";

const sql = new SQL({ url: "postgres://user:pass@localhost:5432/mydb" });
const runner = new Runner(sql);
```

Any other client works as long as it implements the `DB` interface.

### Typed AQL queries — `runner.query`

Compiled `.aql` files are exposed as typed methods under `runner.query`:

```ts
// list_post.aql → listPost
const posts = await runner.query.listPost();
// posts: ListPostRow[]

// get_user.aql with params
const user = await runner.query.getUser({ id: "..." });
// user: GetUserRow | null
```

Each method's param and row types live in the corresponding `<query_name>.ts` file and are re-exported from `runner.ts`.

### Fluent select builder — `runner.select()`

For ad-hoc queries, `runner.select()` returns a typed builder. The shape argument controls which fields are returned and is inferred at compile time.

```ts
// Select specific fields — return type is inferred from the shape
const users = await runner
  .select("User", { id: true, email: true, name: true })
  .all();
// users: Array<{ id: string; email: string; name: string | null }>
```

#### Filtering

`.where()` returns a `FilterChain`. Chain `.and()` and `.or()` on it:

```ts
const users = await runner
  .select("User", { id: true, email: true })
  .where("active", "=", true)
  .and("age", ">=", 18)
  .or("email", "=", "admin@example.com")
  .all();
```

`.and()` and `.or()` are only available after `.where()` — calling them directly on `runner.select()` is a compile-time error.

#### Nested shapes (links)

Pass another builder as a shape value to pull related rows as a JSON array in a single query:

```ts
const users = await runner
  .select("User", {
    id: true,
    email: true,
    posts: runner.select("Post", { title: true, content: true }),
  })
  .all();
// users: Array<{ id: string; email: string; posts: Array<{ title: string; content: string }> }>
```

To filter the sub-select, call `.where()` on the inner builder before passing it:

```ts
const users = await runner
  .select("User", {
    id: true,
    posts: runner
      .select("Post", { title: true })
      .where("authorId", "=", "`User.id`"),  // backtick = outer-query reference
  })
  .all();
```

The backtick syntax (`` "`User.id`" ``) is a correlated reference — Axel resolves it to the outer query's alias at SQL-build time, producing a `WHERE p.author = u.id` condition with no extra round-trips.

#### `.all()` vs `.one()`

```ts
const all  = await runner.select("User", { id: true }).all();   // User[]
const one  = await runner.select("User", { id: true }).where("id", "=", id).one(); // User | null
```

### Insert builder — `runner.insert()`

```ts
const user = await runner
  .insert("User", { email: "alice@example.com", age: 30 })
  .one();
// user: User
```

---

## Go generator (`-g go`)

### Generated files

| File | Contents |
|------|----------|
| `models.go` | One struct per concrete ASL type; enum const blocks |
| `<query_name>.go` | Typed function, params struct, and row struct per AQL query |
| `runner.go` | `Runner` + `Queries` structs with schema embedded for dynamic `Run()` |

### Type mapping

| AQL type   | Go type       | Nullable Go type |
|------------|---------------|------------------|
| `str`      | `string`      | `*string`        |
| `int16`    | `int16`       | `*int16`         |
| `int32`    | `int32`       | `*int32`         |
| `int64`    | `int64`       | `*int64`         |
| `float32`  | `float32`     | `*float32`       |
| `float64`  | `float64`     | `*float64`       |
| `bool`     | `bool`        | `*bool`          |
| `uuid`     | `string`      | `*string`        |
| `datetime` | `time.Time`   | `*time.Time`     |
| `json`     | `interface{}` | `interface{}`    |

### Options

| Option    | Default     | Description |
|-----------|-------------|-------------|
| `package` | `generated` | Package name for all generated files |

```sh
axel codegen -g go -o ./gen --option package=myapp
```

### Setup

```go
import (
    "context"
    "database/sql"
    _ "github.com/lib/pq"

    gen "myapp/gen"
)

db, _ := sql.Open("postgres", "postgres://user:pass@localhost:5432/mydb?sslmode=disable")
runner := gen.NewRunner(db)
```

### Typed AQL queries — `runner.Query`

Compiled `.aql` files are exposed as typed methods under `runner.Query`:

```go
// list_post.aql → Query.ListPost
posts, err := runner.Query.ListPost(ctx)
// posts: []ListPostRow

// get_user.aql with params
user, err := runner.Query.GetUser(ctx, GetUserParams{ID: "..."})
// user: *GetUserRow
```

### Dynamic queries — `runner.Run()`

`Run` compiles and executes any AQL string at runtime, returning `[]map[string]any`. JSON columns (nested shapes, `json_agg` results) are automatically decoded.

```go
rows, err := runner.Run(ctx, `select User { id, email } filter .active = true`, map[string]any{})
if err != nil {
    log.Fatal(err)
}
for _, row := range rows {
    fmt.Println(row["id"].(string), row["email"].(string))
}
```

Pass parameters by name; they are matched to `$name` placeholders in the AQL:

```go
rows, err := runner.Run(ctx,
    `select User { id, email } filter .email = $email`,
    map[string]any{"email": "alice@example.com"},
)
```

---

## Writing a custom generator

Generators can be written in **any language**. Axel invokes an external binary, sends a `CodegenRequest` as JSON on stdin, and expects a `CodegenResponse` as JSON on stdout.

```sh
axel codegen --plugin ./my-generator -o ./gen
```

### Protocol

**Stdin → `CodegenRequest`**

```json
{
  "schema": { ... },
  "queries": [ ... ],
  "config": {
    "out_dir": "./gen",
    "options": { "key": "value" }
  }
}
```

**Stdout ← `CodegenResponse`**

```json
{
  "files": [
    { "path": "models.py", "content": "..." },
    { "path": "queries.py", "content": "..." }
  ]
}
```

All `path` values are relative to `out_dir`. Axel writes the files after the subprocess exits. Anything written to **stderr** is forwarded to the terminal. A non-zero exit code is treated as an error.

### `CodegenRequest` schema

```ts
interface CodegenRequest {
  schema: SchemaDescriptor;
  queries: QueryDescriptor[];
  config: {
    out_dir: string;
    options: Record<string, string>;
  };
}

interface SchemaDescriptor {
  scalars: ScalarDescriptor[];
  enums:   EnumDescriptor[];
  types:   TypeDescriptor[];
}

interface ScalarDescriptor {
  name:     string;  // e.g. "EmailStr"
  base:     string;  // e.g. "str"
  sql_type: string;  // e.g. "TEXT"
}

interface EnumDescriptor {
  name:   string;
  values: string[];
}

interface TypeDescriptor {
  name:        string;
  table:       string;      // SQL table name; empty for abstract types
  is_abstract: boolean;
  extends?:    string[];
  properties?: PropertyDescriptor[];
  links?:      LinkDescriptor[];
  computed?:   ComputedDescriptor[];
  indexes?:    IndexDescriptor[];
}

interface PropertyDescriptor {
  name:         string;
  column:       string;
  aql_type:     string;    // e.g. "str", "int32", "datetime"
  sql_type:     string;    // e.g. "TEXT", "INTEGER", "TIMESTAMPTZ"
  is_required:  boolean;
  is_multi:     boolean;
  default?:     string;
  constraints?: { name: string; args?: string[] }[];
}

interface LinkDescriptor {
  name:             string;
  target_type:      string;
  join_column?:     string;  // FK column name (single link)
  junction_table?:  string;  // Junction table name (multi link)
  is_required:      boolean;
  is_multi:         boolean;
}

interface ComputedDescriptor {
  name: string;
  expr: string;  // SQL expression template
}

interface IndexDescriptor {
  columns: string[];
}
```

### `QueryDescriptor` schema

```ts
interface QueryDescriptor {
  name:       string;    // camelCase function name, e.g. "listPost"
  file:       string;    // source .aql file path
  sql:        string;    // compiled parameterized SQL
  operation:  "select" | "insert" | "update" | "delete";
  params?:    ParamDescriptor[];
  result:     ResultDescriptor;
}

interface ParamDescriptor {
  name:     string;  // e.g. "email"
  aql_type: string;  // e.g. "str"
  sql_pos:  number;  // 1-based $N position in the SQL string
}

interface ResultDescriptor {
  fields?:     ResultField[];
  is_multiple: boolean;  // true → array result
  is_scalar:   boolean;  // true → count/aggregate, no fields
}

interface ResultField {
  name:          string;
  aql_type?:     string;
  sql_type?:     string;
  is_nullable:   boolean;
  is_multiple:   boolean;   // true → JSON array (multi-link or computed sub-select)
  target_type?:  string;    // set for link fields
  sub_fields?:   ResultField[];
}
```

### Example: Python generator

```python
#!/usr/bin/env python3
import json, sys

req = json.load(sys.stdin)
schema = req["schema"]
queries = req["queries"]

files = []

# Generate models
lines = ["# Auto-generated by axel\nfrom typing import Optional, Any\n"]
for typ in schema["types"]:
    if typ["is_abstract"]:
        continue
    lines.append(f"class {typ['name']}:")
    for prop in typ.get("properties", []):
        py_type = {"str": "str", "int32": "int", "bool": "bool"}.get(prop["aql_type"], "Any")
        if not prop["is_required"]:
            py_type = f"Optional[{py_type}]"
        lines.append(f"    {prop['name']}: {py_type}")
    lines.append("")

files.append({"path": "models.py", "content": "\n".join(lines)})

# Generate query stubs
for q in queries:
    params = ", ".join(p["name"] for p in q.get("params", []))
    lines = [
        "# Auto-generated by axel",
        f"SQL = \"\"\"\n{q['sql']}\n\"\"\"",
        "",
        f"def {q['name']}(db{', ' + params if params else ''}):",
        f"    return db.execute(SQL{', [' + params + ']' if params else ''})",
    ]
    files.append({"path": f"{q['name']}.py", "content": "\n".join(lines)})

json.dump({"files": files}, sys.stdout)
```

Make the script executable and point `--plugin` at it:

```sh
chmod +x ./gen.py
axel -d ./myproject codegen --plugin ./gen.py -o ./gen
```

### Example: Go native generator

Native Go generators implement the `codegen.Generator` interface and self-register via `init()`. This is how the built-in `go` and `ts` generators work.

```go
package mygen

import (
    "fmt"
    "bytes"

    "github.com/struckchure/axel/core/codegen"
)

func init() {
    codegen.Register(&MyGenerator{})
}

type MyGenerator struct {
    buf bytes.Buffer
}

func (g *MyGenerator) Name() string { return "mygen" }

func (g *MyGenerator) BeginSchema(_ *codegen.Context, _ codegen.SchemaDescriptor) error {
    g.buf.Reset()
    return nil
}

func (g *MyGenerator) BeginType(_ *codegen.Context, t codegen.TypeDescriptor) error {
    if !t.IsAbstract {
        fmt.Fprintf(&g.buf, "type %s struct {\n", t.Name)
    }
    return nil
}

func (g *MyGenerator) OnProperty(_ *codegen.Context, p codegen.PropertyDescriptor) error {
    fmt.Fprintf(&g.buf, "\t%s string\n", p.Name)
    return nil
}

func (g *MyGenerator) EndType(_ *codegen.Context) error {
    g.buf.WriteString("}\n\n")
    return nil
}

func (g *MyGenerator) EndSchema(ctx *codegen.Context) error {
    return ctx.WriteFile("models.xyz", g.buf.Bytes())
}

// Unused hooks — must still be implemented.
func (g *MyGenerator) OnScalar(_ *codegen.Context, _ codegen.ScalarDescriptor) error  { return nil }
func (g *MyGenerator) OnEnum(_ *codegen.Context, _ codegen.EnumDescriptor) error      { return nil }
func (g *MyGenerator) OnLink(_ *codegen.Context, _ codegen.LinkDescriptor) error      { return nil }
func (g *MyGenerator) OnComputed(_ *codegen.Context, _ codegen.ComputedDescriptor) error { return nil }
func (g *MyGenerator) OnIndex(_ *codegen.Context, _ codegen.IndexDescriptor) error    { return nil }
func (g *MyGenerator) OnQuery(_ *codegen.Context, _ codegen.QueryDescriptor) error    { return nil }
```

Register it with a blank import in your `cmd/` package (after forking the repo or embedding Axel as a library):

```go
import _ "myapp/generators/mygen"
```

Then use it like any built-in:

```sh
axel -d ./myproject codegen -g mygen -o ./gen
```

### Hook call order

```
BeginSchema
  OnScalar   (each custom scalar, alphabetical)
  OnEnum     (each enum, alphabetical)
  BeginType  (each type, alphabetical — abstract types included)
    OnProperty / OnLink / OnComputed / OnIndex  (each member, declaration order)
  EndType
  OnQuery    (each AQL query, in discovery order)
EndSchema
```

Use `BeginSchema` to reset state, `BeginType`/`EndType` to open and close type-level buffers, and `EndSchema` to flush everything to files via `ctx.WriteFile`.

`ctx.WriteFile(path, content)` writes `content` to `<out_dir>/<path>`, creating parent directories as needed. Paths are relative to `out_dir`.
