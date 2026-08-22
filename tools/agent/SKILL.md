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

## ASL essentials

```asl
use extension 'pgcrypto';
use extension 'postgis';
use extension 'vector';

scalar type EmailStr extends str;
scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32;
  longitude: float32;
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
  required link author: User;      # single link  → author_id FK column
  multi link tags: Tag;            # many-to-many → junction table

  policy owner_only for all using ( .author = global current_user );
}
```

Things that reliably trip people up:

- **`abstract type` produces no table.** It exists to be inherited; its members are flattened into
  every child. A concrete `type` produces a table named in snake_case (`BlogPost` → `blog_post`).
- **`link` vs `multi link`.** A single link becomes a nullable-or-not FK column on *this* table. A
  `multi link` becomes a junction table — Axel creates and manages it. There is no "belongs to /
  has many" pair to declare on both sides.
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

# Computed field in a shape
multi select User { email, upper_email := upper(.email) };

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

Full grammar — `with` blocks, sub-selects, casts, `group by`/`having`, conflict targets:
**`references/aql.md`**.

## Verify, don't assume

The compiler is the source of truth and it is fast. Prefer running it over reasoning about it:

```bash
axel fmt -w .                                    # format all .asl / .aql files in place
axel validate                                    # schema resolves?
axel compile --aql 'select User { id }'          # what SQL does this shape produce?
axel diff -n "wip" && cat migrations/*/up.sql    # what DDL does this change produce?
```

Read the SQL that comes back. If it is not what the user wanted, the fix belongs in the `.asl` or
`.aql` source, never in the output.

When something fails, the error usually names the construct and (for schema-wide problems) the file
and line. `axel validate` errors are resolve-time — an unknown type, a link to a type that does not
exist, a redeclared name, an inheritance cycle. `axel compile` errors are query-time — an unknown
field on a shape, a filter on a column that is not there.

## Common corrections

| Symptom | Cause |
|---|---|
| `axel diff` writes an empty migration | The schema did not actually change, or `schema-path` points somewhere else than you edited |
| Migration wants to `DROP` and re-`CREATE` a column | A rename read as delete-plus-add; Axel diffs structure, not intent |
| `unknown type "Foo"` | Typo, or the file declaring `Foo` is not covered by `schema-path` |
| `declared more than once` | The same name in two files of a split schema — they share one namespace |
| A `multi link` produced no column | Correct: it produced a junction table |
| Query returns only one row | A plain `select`; use `multi select` |
| `limit/offset require 'multi select'` | Same cause — add `multi` |
| `type "User" has no field "posts"` | Reverse links do not exist; query from the side holding the link |
| Inline query in the shell loses its parameters | Double quotes; use single quotes |

## Reference files

- `references/asl.md` — the full schema language: every declaration, field body item, constraint,
  trigger, policy, function directive, and the SQL each lowers to.
- `references/aql.md` — the full query grammar, with the compiled SQL shape for each construct.
- `references/cli.md` — every command and flag, the config file, and codegen.
