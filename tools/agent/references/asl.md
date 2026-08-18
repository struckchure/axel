# ASL reference

Axel Schema Language. One or more `.asl` files describe a PostgreSQL schema; `axel diff` lowers the
difference between the schema and the last migration into `up.sql` / `down.sql`.

Comments start with `#`. Every top-level declaration and every member inside a type body ends with
`;` (a type body's closing `}` does not).

## Top-level declarations

```asl
use extension 'pgcrypto';              # CREATE EXTENSION IF NOT EXISTS
scalar type EmailStr extending str;    # named alias over a builtin scalar
enum Role { Admin, Member, Guest }     # TEXT column + CHECK constraint
global current_user: uuid;             # session GUC, not DDL
global required tenant: str;
abstract type Base { … }               # no table; inherited only
type User extending Base { … }         # table "user"
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
| `float32` | `REAL` | | `json` | `JSONB` |
| `float64` | `DOUBLE PRECISION` | | `bytes` | `BYTEA` |
| `bool` | `BOOLEAN` | | `decimal` | `NUMERIC` |

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
required link author: User;   # FK column author → user.id, NOT NULL
link editor: User;            # nullable FK
multi link tags: Tag;         # junction table "post_tags"
```

A single link is a column on *this* table. A `multi link` is a junction table Axel creates and
manages. Links are one-directional: the target type gains nothing, and AQL has no reverse-link
traversal.

### Computed fields

```asl
computed display_name := .name ?? .email;
```

Not stored. The expression is expanded into the SQL at query-compile time, so it can be selected in
an AQL shape but not indexed.

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
| `inheritance cycle detected involving type "X"` | `extending` loops back on itself |
| `type "X" declared more than once (a.asl:1:1 and b.asl:4:1)` | One flat namespace across files |
| `global "g": unknown type … (globals must be a scalar type)` | Globals cannot be object-typed |
| `type "X" trigger "t" executes unknown function "f"` | Trigger target missing or not `-> trigger` |
| `type "X": link "l" references unknown type "Y"` | Link target does not exist |
| `policy "p": \`using\` is not allowed for insert` | Split it: `using` for select/update/delete, `with check` for insert |
