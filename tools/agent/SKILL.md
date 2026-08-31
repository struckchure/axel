---
name: axel
description: >-
  Write PostgreSQL schemas in ASL and queries in AQL with Axel
  (github.com/struckchure/axel) — the ahead-of-time compiler that turns .asl into migration SQL and
  .aql into parameterized query strings. Use this whenever work touches .asl or .aql files, an
  axel.yaml config, the `axel` CLI (validate, diff, up, down, status, compile, codegen, fmt, studio,
  lsp), or generated Axel clients. It applies even when the user never mentions Axel by name — if
  the project has an axel.yaml, a schema.asl, or a schema/ directory of .asl files, then "add a
  users table", "make email unique", "write a query for posts with their author", "why is my
  migration empty", "generate the Go client", or "add row-level security" all belong here. Also use
  it for modelling questions (links vs multi links, abstract types, triggers, policies, globals) and
  for debugging resolve, compile, or migration errors.
---

# Axel

Axel compiles; it does not execute. `.asl` files describe a PostgreSQL schema and compile to
migration SQL. `.aql` files describe query shapes and compile to parameterized SQL strings that
**you** run with your own driver (`pgx`, `node-postgres`, `sqlx`). There is no runtime, no
connection pool, no lazy loading. Nothing happens at query time that is not visible in the
generated SQL.

That framing decides most questions. If someone asks "how do I execute this query with Axel" the
answer is: you don't — you compile it and hand the SQL to your driver.

## Orient before writing anything

```bash
ls axel.yaml 2>/dev/null && cat axel.yaml       # schema-path, migrations-dir, codegen settings
axel validate                                    # does the current schema even resolve?
ls migrations/ 2>/dev/null | tail -5             # how far along are the migrations?
```

`schema-path` may be a single file, a directory, or a glob (`schema/*.asl`) — a schema is often
split across files that share one flat namespace. Read the whole set before editing, and match the
conventions already there (an abstract `Base` type is a near-universal pattern).

If `axel` is not on PATH, say so rather than guessing at output: every verification step below needs
the binary.

## The loop

Editing a schema or query is always the same cycle, and every step is cheap:

```bash
axel fmt -w .                   # canonical formatting (run after EVERY edit to .asl / .aql)
axel validate                   # parse + resolve + validate; no database needed
axel diff -n "add comments"     # write migrations/NNNN/{up,down}.sql from the change
axel up                         # apply pending migrations (needs a database)
axel compile -f queries/x.aql   # see the SQL a query compiles to
axel codegen -g go -o ./gen     # regenerate the typed client
```

**Always format after modifying `.asl` or `.aql` files.** Run `axel fmt -w .` (or `axel fmt -w <path>`) so code matches Axel's canonical style before validation, diffing, or committing.

**Run `axel validate` after every schema edit.** It is fast, needs no database, and catches the
entire class of errors that otherwise surface as a confusing migration. Then run `axel diff` and
*read the generated `up.sql`* before applying it — the diff is the review surface, and a surprising
`DROP` there is the signal that a rename was read as a delete-plus-create.

Never hand-edit a file under `migrations/`. Change the schema and re-diff.

**Adding a `required` field to a populated table needs a backfill.** `ADD COLUMN … NOT NULL` fails
on a table that already has rows, so Axel adds the column nullable, leaves a commented backfill seam,
and raises `NOT NULL` in a separate statement:

```sql
ALTER TABLE "vendor" ADD COLUMN "description" TEXT;
-- axel: "description" is required and has no default. Existing rows need a value
-- before the NOT NULL below can be applied:
--   UPDATE "vendor" SET "description" = <value> WHERE "description" IS NULL;
ALTER TABLE "vendor" ALTER COLUMN "description" SET NOT NULL;
```

`axel diff` also prints a warning naming the column. Fill that `UPDATE` in, or the migration fails on
any non-empty table. Declaring a `default` avoids the whole problem: Axel then knows the backfill
value, writes the `UPDATE` itself, and keeps `NOT NULL` inline. Flipping an existing optional field
to `required` has the same hazard and gets the same treatment.

## ASL essentials

```asl
use extension 'pgcrypto';
use extension 'postgis';
use extension 'vector';

scalar type EmailStr extends str;
scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32 := ST_Y(__self__::geometry);
  longitude: float32 := ST_X(__self__::geometry);
};
scalar type Embedding extends sql "vector(1536)" as multi float32;
scalar type Citext extends sql "citext" as str;

enum Role { Admin, Member, Guest }
global current_user: uuid;

abstract type Base {
  required id: uuid { default := gen_uuid(); constraint pk; };
  required created_at: datetime { default := datetime_current(); };
  required updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();
  };
}

type User extends Base {
  required email: EmailStr { constraint exclusive; };
  name: Citext { default := 'n/a'; };
  location: Point;
  bio_vec: Embedding;
  required role: Role;
  active: bool { default := true };

  computed display_name := .name ?? .email;
  index on (.email);
}

type Post extends Base {
  required title: str;
  required link author: User;      # single link  → FK column named "author"
  multi link tags: Tag;            # many-to-many → junction table "post_tags"
  multi keywords: str;             # multi SCALAR → a TEXT[] column, not a junction

  policy owner_only for all using ( .author = global current_user );
}
```

Things that reliably trip people up:

- **`abstract type` produces no table.** It exists to be inherited; its members are flattened into
  every child. A concrete `type` produces a table named in snake_case (`BlogPost` → `blog_post`).
- **`link` vs `multi link`.** A single link becomes a nullable-or-not FK column on *this* table,
  named after the **field** — `link author: User` gives a column `author`, not `author_id`. A
  `multi link` becomes a junction table `{owner}_{field}` (`post_tags`) whose two FK columns are
  named after the tables they reference (`post`, `tag`). There is no "belongs to / has many" pair to
  declare on both sides.
- **A self-referential `multi link` names its target column after the field.** Both junction columns
  would otherwise be called `product`, which Postgres rejects, so `multi link addons: Product` on
  `Product` yields `product_addons("product", "addons")`.
- **`multi` on a scalar is an array column, not a link.** `multi roles: UserType` is one `TEXT[]`
  column. Membership against it compiles to `= ANY(...)` (Postgres `IN` wants a parenthesised list);
  membership against a `multi link` compiles to an `EXISTS` over the junction table. Delta
  assignment (`{ "+": …, "-": … }`) applies only to multi links — a multi scalar is replaced whole.
- **Custom SQL extension types.** Declare scalars using `scalar type Point extends sql "geography(Point, 4326)" as { ... };` or `as multi <Type>` or `as <Type>`. This maps to PostgreSQL DDL while providing client typing in codegen and dot-access in AQL.
- **Every member ends with `;`**, including the closing brace of a field body: `name: str { … };`.
- **`required` is a prefix**, not a modifier in the body: `required email: str;`.
- **Order never matters.** A type may extend a parent declared below it, or in another file; a link
  may target a type defined anywhere in the set.
- **Names are global across the whole schema.** Declaring the same type, enum, scalar, global or
  function twice is an error that names both file locations.

Full grammar — field bodies, constraints, partial unique constraints, rewrites, triggers, policies,
functions, globals: **`references/asl.md`**.

## AQL essentials

**A plain `select` returns one row.** Axel appends an implicit `LIMIT 1` and codegen produces a
single-row result type. Prefix with `multi` for a set — and `limit`/`offset` are only accepted on a
`multi select`. This is the single most common mistake:

```aql
select User { id, email } filter .id = $id;                 # one row, implicit LIMIT 1
multi select User { id, email } order by .email limit 10;   # all matching rows
```

```aql
# Nested shape → one query, json_agg, no N+1
multi select Post {
  title,
  author: { id, email },
  tags: { name }
} filter .author.role = Role.Admin order by .created_at desc limit 5;

# Computed field in a shape (functions, arithmetic, coalesce)
multi select User { email, upper_email := upper(.email) };
multi select OrderItem { id, subtotal := .unit_price * .quantity, total := (.unit_price * .quantity) - .discount };

# Aggregate select — its own form; it may not be mixed with row fields
select count(User filter .active = true);

# Aggregate shape with expressions (e.g. min/sum/avg/count with math or func calls)
select Place {
  nearest := min(haversine(.location.latitude, .location.longitude, $target_lat, $target_lon))
};

# …or as a scalar subquery correlated to the outer row
multi select User { id, email }
  filter (select count(Post filter .author.id = User.id)) > 0;

insert User { email := $email, role := Role.Member }
  unless conflict on .email
  else ( update User set { role := Role.Member } );

update Post filter .id = $id set { title := $title };

# Multi-link updates: delta (+ and -) or full set replacement
update Organization filter .id = $id set {
  members := {
    "+": (multi select User filter .email in $invite_emails),
    "-": (select User filter .id = $removed_user)
  }
};
update Organization filter .id = $id set {
  members := (multi select User filter .active = true)
};

delete Post filter .created_at < $cutoff;
```

- Leading `.` means "a field of the type being queried" (`.email`); `$name` is a parameter. The
  compiled SQL is positional (`$1`, `$2`) with a comment header naming and typing each one.
- A nested shape (`author: { … }`) compiles to a correlated `json_agg` / `row_to_json` subquery, or
  a `LEFT JOIN LATERAL` when `rel-load-strategy: join` is set in `axel.yaml`. Either way it is one
  round trip.
- There are **no reverse links**: `User` has no `.posts` just because `Post` has `link author: User`.
  Query from the side that owns the link, or use a correlated aggregate as above.
- Enum values are qualified: `Role.Admin`.
- `and` binds tighter than `or`; parenthesize when mixing.
- Always single-quote an inline query in the shell (`axel compile --aql '…'`) or the shell eats
  `$params`.

### Parameters

A parameter's type is inferred from what it is compared to or assigned to. Declare it explicitly
when there is nothing to infer from (`limit`/`offset`), when it must be `multi`, or when you want
one declaration reused across the whole query. A leading `var` block is the place for that; `:type`
and `<type>` are the same annotation:

```aql
var (
  multi $ids: uuid;        # ONE array bind, not N params
  $status<Role>?;          # optional: skipped when null
  $limit: int32? := 20;    # optional with a default
)

multi select User { id, email }
filter .id in $ids and .role = $status
limit $limit;
```

```sql
WHERE u.id = ANY($1::UUID[]) AND ($2::TEXT IS NULL OR u.role = $2::TEXT)
LIMIT COALESCE($3::INTEGER, 20)
```

- **`multi $x` binds a single array**, so `in $x` lowers to `= ANY($1::T[])`, never `IN`. The element
  type is inferred from the compared column if the declaration omits it. Clients type it as an array
  (`string[]`, `[]string`) — one value, not a spread.
- **`?` means skip-when-null**, whether declared in the `var` block or written inline at the use site
  (`$email?`). In an `and` context an omitted param matches everything; inside an `or` it drops its
  own arm out instead of voiding the other arms.
- **A declared default is coalesced, not skipped.** `$age: int32? := 21` compiles to
  `COALESCE($1, 21)` and the comparison still runs — otherwise the default would be silently ignored.
- Optional array params keep the array type in **every** cast of the placeholder
  (`($1::TEXT[] IS NULL OR u.email = ANY($1::TEXT[]))`); casting one placeholder to both `T` and
  `T[]` makes Postgres reject the statement.

### Unknown functions warn, they do not fail

Function pass-through is the escape hatch for arbitrary SQL, so an unrecognised name still compiles
and `axel compile` / `axel codegen` print a warning on stderr (the LSP surfaces it as a warning
diagnostic). Read those warnings — they are the difference between a query that works and one that
compiles cleanly and only fails against a real database:

```
warning: "distinct" is a SQL keyword, not a function; Postgres will reject distinct(...)
```

Full grammar — `with` blocks, sub-selects, casts, `group by`/`having`, conflict targets:
**`references/aql.md`**.

## Client and codegen essentials

Axel emits typed clients for Go (`-g go`) and TypeScript (`-g ts`):

```bash
axel codegen -g go -o ./gen --option package=gen
axel codegen -g ts -o ./gen
```

- **Query execution**: Compiled `.aql` files generate typed methods (`runner.Query.GetUser(ctx, params)` in Go, `runner.query.getUser(params)` in TypeScript). **Always prefer compiled queries or query builders over runtime `runner.run(...)`.**
- **Transactions & Custom Connections**: Pass transactions directly using `runner.WithDB(tx)` / `gen.NewQueries(tx)` in Go, or `runner.withDb(tx)` in TypeScript.
- **Fluent query builders (TypeScript)**: Build runtime shapes and queries with `runner.select()`, `runner.insert()`, `runner.update()`.
- **Custom SQL & PostGIS types in builders**: When inserting or updating custom extension types (`geography`, `geometry`, `vector`), pass values in their native PostgreSQL input text representation (e.g. EWKT format `"SRID=4326;POINT(lng lat)"` for geography, `"[1.0, 2.0]"` for vector, ISO strings for timestamps). PostgreSQL coerces untyped parameters to the target column type automatically at execution time without requiring SQL function calls in value positions.
- **Runtime `runner.run(...)` limitations**: The client-bundled runtime parser is intentionally narrower than `axel compile`. It does **not** support `var (...)` blocks, SQL function calls in value positions (like `ST_MakePoint(...)`), or complex AST transforms. Queries that fail in `runner.run` will fail at **runtime** (not build-time).
- **Session globals**: Scoped via `runner.With<Global>(...)` (Go) / `runner.with<Global>(...)` (TypeScript), or functional options on standalone functions.
- **Multi links in the TypeScript client** appear on the model as a branded `Relation<T>` field. The
  brand is what lets `Insertable`/`Updatable` strip them — a junction row cannot be written through
  an `INSERT` column list — so write them with an `update … set { members := … }` query, not as an
  insert column. Selecting one through the fluent builder is supported; *sub-shapes* on a link
  (`author: { id, email }`) remain AQL-only.
- **`multi` scalars type as arrays** on both sides (`string[] | null`, `[]string`), with the
  nullability outside the array.
- **Temporal fields inside a typed JSON scalar are strings.** Postgres renders `date`/`time`/
  `datetime` into JSON as text and neither `encoding/json` nor `JSON.parse` revives them, so the
  generators map them to `string`, not `time.Time` / `Date`. Parse them yourself at the edge.
- **TypeScript codegen emits an `index.ts` barrel** re-exporting the generated modules.

Full codegen guide — generated files, options, transaction patterns, and plugin protocol:
**`references/codegen.md`**.

## Verify, don't assume

The compiler is the source of truth and it is fast. Prefer running it over reasoning about it:

```bash
axel fmt -w .                                    # format all .asl / .aql files in place
axel validate                                    # schema resolves?
axel compile --aql 'select User { id }'          # inspect compiled SQL inline
axel compile -f queries/x.aql -o queries/x.sql   # compile a specific query to a file
axel diff -n "wip" && cat migrations/*/up.sql    # what DDL does this change produce?
```

> **Warning:** Do not run `axel compile -d .` without `--output-dir`, as it will compile every `.aql` query and write `.sql` files directly into your project working directory. Use `axel compile --aql '<query>'` for inline inspection or specify `--output-dir`.

Read the SQL that comes back. If it is not what the user wanted, the fix belongs in the `.asl` or
`.aql` source, never in the output.

When something fails, the error usually names the construct and (for schema-wide problems) the file
and line. `axel validate` errors are resolve-time — an unknown type, a link to a type that does not
exist, a redeclared name, an inheritance cycle. `axel compile` errors are query-time — an unknown
field on a shape, a filter on a column that is not there.

## Common corrections

| Symptom | Cause | Fix |
|---|---|---|
| `axel diff` writes an empty migration | The schema did not actually change, or `schema-path` points somewhere else than you edited | Check `axel.yaml` `schema-path` |
| Migration wants to `DROP` and re-`CREATE` a column | A rename read as delete-plus-add; Axel diffs structure, not intent | Check `up.sql` and write rename migration explicitly if needed |
| `unknown type "Foo"` | Typo, or the file declaring `Foo` is not covered by `schema-path` | Fix typo or check `schema-path` glob |
| `declared more than once` | The same name in two files of a split schema — they share one namespace | Rename duplicate declaration |
| A `multi link` produced no column | Correct: it produced a junction table | Expected behavior |
| Query returns only one row | A plain `select`; use `multi select` | Change `select` to `multi select` |
| `limit/offset require 'multi select'` | Same cause — add `multi` | Add `multi` prefix |
| `type "User" has no field "posts"` | Reverse links do not exist; query from the side holding the link | Query from the model owning the `link` |
| Inline query in the shell loses its parameters | Double quotes; use single quotes | Single-quote the query string: `'select ... $param'` |
| Runtime error `expected keyword, got {"k":"id","v":"var"}` or `expected id got "("` in `runner.run` | `runner.run` runtime parser does not support `var` blocks or function calls (e.g. `ST_MakePoint`) in value positions | Use generated query builder (`runner.insert`) or compiled `.aql` file; pass custom types as EWKT strings (`"SRID=4326;POINT(lng lat)"`) |
| Unwanted `.sql` files generated in project root | Ran `axel compile -d .` without `--output-dir` | Use `axel compile --aql '...'` for inspection, or supply `--output-dir` |
| Postgres rejects `IN` against an array column | `in` on a `multi` scalar or a `multi` param needs `= ANY`, which Axel emits — an old build, or hand-written SQL | Recompile; check the emitted SQL says `= ANY(...)` |
| `column "x" is of type uuid[] but expression is of type uuid` | A `multi` param was passed one element per value instead of a single array | Pass one array; `multi $ids` is *one* bind |
| Migration fails with `column contains null values` | A `required` field with no default was added to a populated table | Fill in the `UPDATE … SET … WHERE … IS NULL` seam in `up.sql`, or declare a `default` |
| Query compiles but the database rejects the function | Unknown functions pass through by design | Read the `warning:` line from `axel compile`/`codegen` |
| `column "product" appears twice in primary key constraint` | A self-referential multi link on an old build named both junction columns after the table | Recompile; the target column now falls back to the field name |
| A JSON field's timestamp arrives as a string | Postgres renders temporal values inside JSON as text; the generators type them `string` | Parse it at the edge — this is intended |
| A multi link is missing from a TypeScript insert | Multi links are branded `Relation<T>` and stripped from `Insertable` | Set it with an `update … set { … }` after the insert |

## Reference files

- `references/asl.md` — the full schema language: every declaration, field body item, constraint,
  trigger, policy, function directive, and the SQL each lowers to.
- `references/aql.md` — the full query grammar, with the compiled SQL shape for each construct.
- `references/codegen.md` — client code generation for Go and TypeScript, transactions, custom types, and custom plugins.
- `references/cli.md` — every command and flag, the config file, and codegen.
