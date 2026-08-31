# ASL reference

Axel Schema Language. One or more `.asl` files describe a PostgreSQL schema; `axel diff` lowers the
difference between the schema and the last migration into `up.sql` / `down.sql`.

Comments start with `#`. Every top-level declaration and every member inside a type body ends with
`;` (a type body's closing `}` does not).

## Top-level declarations

```asl
use extension 'pgcrypto';              # CREATE EXTENSION IF NOT EXISTS
use extension 'postgis';
use extension 'vector';

scalar type EmailStr extends str;      # named alias over a builtin scalar
scalar type Code extends str {         # extended scalar with field descriptors
  constraint min_length(6);
  constraint max_length(6);
  default := '000000';
}
scalar type Coordinate extends json {  # typed JSON scalar
  lat: str;
  lng: str;
}
# Custom PostgreSQL Extension Types with client codegen typing:
scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32 := ST_Y(__self__::geometry);
  longitude: float32 := ST_X(__self__::geometry);
};
scalar type Embedding extends sql "vector(1536)" as multi float32;
scalar type Citext extends sql "citext" as str;
scalar type Geometry extends sql "geometry";

enum Role { Admin, Member, Guest }     # TEXT column + CHECK constraint
global current_user: uuid;             # session GUC, not DDL
global required tenant: str;
abstract type Base { … }               # no table; inherited only
type User extends Base { … }           # table "user"
model User { … }                       # `model` is accepted as an alias for `type`
```

Declaration order is irrelevant, within a file and across files. Names live in one flat namespace
shared by types, enums and scalars — a duplicate is an error naming both locations.

### Splitting across files

`schema-path` accepts a file, a directory, or a glob (`schema/*.asl`, `schema/**/*.asl`). Matching
files are merged into one schema. There is no `import`; `use extension` is deduplicated.

## Builtin scalars

| ASL | PostgreSQL | | ASL | PostgreSQL |
|---|---|---|---|---|
| `str` | `TEXT` | | `uuid` | `UUID` |
| `int16` | `SMALLINT` | | `datetime` | `TIMESTAMPTZ` |
| `int32` | `INTEGER` | | `date` | `DATE` |
| `int64` | `BIGINT` | | `time` | `TIME` |
| `float32` | `REAL` | | `json` | `JSON` |
| `float64` | `DOUBLE PRECISION` | | `jsonb` | `JSONB` |
| `bool` | `BOOLEAN` | | `bytes` | `BYTEA` |
| `decimal` | `NUMERIC` | | | |

Table and column names are snake_cased from the declaration (`BlogPost` → `blog_post`).

## Type members

### Properties

```asl
required email: str;                    # NOT NULL
name: str;                              # nullable
required property email: str;           # `property` keyword is optional
active: bool { default := true };
required id: uuid {
  default := gen_uuid();                # builtin → gen_random_uuid()
  constraint pk;
  constraint exclusive;                 # UNIQUE
};
title: str {
  constraint min_length(3);
  constraint max_length(200);
};
status: Status { default := Status.Draft };   # qualified enum default
```

Field-body items: `constraint`, `default :=`, `rewrite`, `on`.

### Links

```asl
required link author: User;   # FK column "author" → user.id, NOT NULL
link editor: User;            # nullable FK "editor"
multi link tags: Tag;         # junction table "post_tags" ("post", "tag")
```

A single link is a column on *this* table, named after the **field** — `link author: User` gives a
column `author`, not `author_id`. A `multi link` is a junction table Axel creates and manages, named
`{owner_type}_{field}` in snake_case, with one FK column per side named after the table it
references. Links are one-directional: the target type gains nothing, and AQL has no reverse-link
traversal.

**Self-referential multi links.** When the link points back at its own type, both junction columns
would be named after the same table, which Postgres rejects (`column "product" appears twice in
primary key constraint`). The target side falls back to the field name:

```asl
type Product {
  required id: uuid { constraint pk; };
  multi link addons: Product;   # product_addons("product", "addons")
}
```

Every producer of junction SQL — the DDL generator, the AQL compiler, the generated clients, studio
— derives these names from the same helper, so they always agree.

### Multi scalars (array columns)

`multi` on a *scalar* field means an array column, not a junction table:

```asl
type User {
  multi link teams: Team;   # junction table "user_teams"
  multi roles: Role;        # ONE column "roles" of type TEXT[]
  multi scores: float64;    # DOUBLE PRECISION[]
}
```

The two behave differently in queries: membership against a multi scalar compiles to `= ANY(...)`
(Postgres `IN` takes a parenthesised list, not an array), while membership against a multi link
compiles to an `EXISTS` over the junction table. Delta assignment (`{ "+": …, "-": … }`) applies only
to multi links; a multi scalar is assigned as a whole array. In generated clients a multi scalar
types as an array with the nullability outside it (`Role[] | null`, `[]Role`).

An enum array is a `TEXT[]` guarded by a containment CHECK rather than a Postgres enum array:

```sql
"roles" TEXT[] CONSTRAINT "chk_user_roles_enum" CHECK ("roles" <@ ARRAY['Admin', 'Member']::TEXT[])
```

### Computed fields

```asl
computed display_name := .name ?? .email;
computed total := (.quantity * .unit_price) - .discount;
computed tax := .unit_price * 0.2;
```

Not stored. Supports arithmetic operators (`+`, `-`, `*`, `/`), unary signs (`+`, `-`), function calls, and `??` (null coalescing). The expression is expanded into the SQL at query-compile time, so it can be selected in an AQL shape but not indexed.

### Rewrites

Sugar for a `BEFORE` trigger that assigns the field on the named events (`insert`/`create`,
`update`):

```asl
required updated_at: datetime {
  default := datetime_current();
  rewrite update := datetime_current();
};
slug: str { rewrite insert, update := slugify(__new__.title); };
status: str { rewrite update := __old__.status; };
```

Row references: `__new__`, `__old__`, `__subject__` (alias for `__new__`).

### Indexes and composite constraints

```asl
index on (.email);
index on (.active, .age);
constraint exclusive on (.tenant, .name);
constraint exclusive on (.name, .actor) filter .status = QueueStatus.Pending;   # partial unique index
```

### Triggers

```asl
trigger audit after insert, update, delete do (
  insert AuditLog { table_name := 'post', row_id := __new__.id }
);
trigger touch before update for each row execute my_fn();
trigger guard before update when ($$ OLD.locked $$) execute reject();
```

Two mutually exclusive bodies: inline AQL in `do ( … )`, or `execute <fn>()` naming a declared
function that returns `trigger`. `when ( $$ … $$ )` holds raw SQL.

### Policies (row-level security)

```asl
policy owner_only for all using ( .owner = global current_user );
policy hide_expired for select using ( .expires_at is null or .expires_at >= now() );
policy tenant_rw for update to app_user
  using ( .tenant = global tenant )        # rows this policy can see
  with check ( .tenant = global tenant );  # rows this policy may write
policy tenant_ins for insert with check ( .tenant = global tenant );
```

Declaring any policy enables RLS on the table. Predicates use `.field` for the type's own columns
and `global <name>` for a global. Commands are `select`, `insert`, `update`, `delete`, `all`;
Postgres allows `using` on everything except `insert`, and `with check` only on `insert`, `update`
and `all` — Axel enforces this.

## Functions

```asl
@language plpgsql
@immutable
@strict
function slugify(value: text) -> text {
  return regexp_replace(lower(value), '[^a-z0-9]+', '-', 'g');
};

function touch() -> trigger { return aql`update Post filter .id = $id set { seen := true }`; };
```

Directives are decorator-style and precede the declaration: `@language <lang>`, one of
`@immutable` / `@stable` / `@volatile`, `@strict`, `@leakproof`,
`@parallel <safe|restricted|unsafe>`, `@security <definer|invoker>`, `@cost <n>`, and
`@for <Type>` (run-once-per-type helper). The body is a single
`return <expr>;` — a raw Postgres expression, or a backticked inline `aql` query. Parameter and
return types may be ASL scalars or raw Postgres types, with `[]` for arrays.

## Errors you will see from `axel validate`

| Message | Meaning |
|---|---|
| `unknown type "X"` | Not a builtin, scalar, enum or declared type — or its file is outside `schema-path` |
| `type "X" extends unknown type "Y"` | Parent not declared anywhere in the schema set |
| `inheritance cycle detected involving type "X"` | `extends` loops back on itself |
| `type "X" declared more than once (a.asl:1:1 and b.asl:4:1)` | One flat namespace across files |
| `global "g": unknown type … (globals must be a scalar type)` | Globals cannot be object-typed |
| `type "X" trigger "t" executes unknown function "f"` | Trigger target missing or not `-> trigger` |
| `type "X": link "l" references unknown type "Y"` | Link target does not exist |
| `policy "p": \`using\` is not allowed for insert` | Split it: `using` for select/update/delete, `with check` for insert |
