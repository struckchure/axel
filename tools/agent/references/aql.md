# AQL reference

Axel Query Language. A `.aql` file holds one query shape; `axel compile` turns it into
parameterized SQL, and `axel codegen` turns it into a typed function in Go or TypeScript.

Comments start with `#` (**not** `--`; the lexer rejects `--`). The trailing `;` is optional for an
inline query passed with `--aql`.

## Grammar

```
Statement   = VarBlock* WithBlock? (SelectStmt | InsertStmt | UpdateStmt | DeleteStmt | ForStmt)

VarBlock    = "var" "(" (VarParam ";"?)* ")" | "var" VarParam ";"?
VarParam    = "multi"? "$" Ident (":" Ident | "<" Ident ">")? "?"? (":=" DefaultVal)?
DefaultVal  = SetLiteral | Expr
Param       = "$" Ident (":" Ident | "<" Ident ">")? "?"?

WithBlock   = "with" "(" (WithBinding ";")* ")"
WithBinding = Ident ":=" "(" "multi"? "select" SelectBody ")"

ForStmt     = "for" ("$" Ident | Ident) "in" Expr "{" (InsertStmt | SelectStmt | UpdateStmt | DeleteStmt) "}" ";"?

SelectStmt  = "multi"? "select" SelectBody ";"?
SelectBody  = AggExpr
            | TypeName Shape? Filter? GroupBy? Having? OrderBy? Limit? Offset?
AggExpr     = Ident "(" TypeName Filter? ")"

InsertStmt  = "insert" TypeName "{" Assignment ("," Assignment)* ","? "}" Conflict? ";"?
UpdateStmt  = "update" TypeName Filter? "set" "{" Assignment ("," Assignment)* ","? "}" ";"?
DeleteStmt  = "delete" TypeName Filter? ";"?

Conflict    = "unless" "conflict" ("on" ConflictTarget)? ("else" "(" ConflictUpdate ")")?
ConflictTarget = "." Ident | "(" "." Ident ("," "." Ident)* ")"
ConflictUpdate = "update" TypeName "set" "{" Assignment ("," Assignment)* ","? "}"

Shape       = "{" ShapeField ("," ShapeField)* ","? "}"
ShapeField  = Ident (":" Shape)?          # leaf, or nested link shape
            | Ident ":=" Expr Filter?     # computed or aggregate field
Assignment  = Ident ":=" (LinkDelta | Expr)
LinkDelta   = "{" LinkDeltaItem ("," LinkDeltaItem)* ","? "}"
LinkDeltaItem = (String | "+" | "-") ":" Expr

Filter      = "filter" Expr
GroupBy     = "group" "by" Expr ("," Expr)*
Having      = "having" Expr
OrderBy     = "order" "by" Expr ("asc" | "desc")? ("," …)*
Limit       = "limit" Expr
Offset      = "offset" Expr

Expr        = AndExpr ("or" AndExpr)*        # `and` binds tighter than `or`
AndExpr     = Cmp ("and" Cmp)*
Cmp         = AddExpr (BinOp AddExpr | "is" "not"? "null")?
BinOp       = "=" | "!=" | "<" | "<=" | ">" | ">=" | "??" | "in" | "like" | "ilike"
AddExpr     = MulExpr (("+" | "-") MulExpr)*
MulExpr     = Factor (("*" | "/") Factor)*
Factor      = ("+" | "-")? Primary
Primary     = Operand ("<" Ident ">")?       # trailing cast on any operand
Operand     = "(" "multi"? "select" SelectBody ")" ("." Ident)?   # sub-select
            | "(" "insert" TypeName "{" … ")"                      # sub-insert → id
            | "(" Expr ")" | FuncCall | PathExpr | SetLiteral
            | QualifiedIdent                                       # User.id — outer reference
            | "$" Ident | "global" Ident | "null" | "true" | "false"
            | String | Int | Float | Ident
SetLiteral  = "{" (Expr ("," Expr)* ","?)? "}"
PathExpr       = ("." Ident)+
QualifiedIdent = Ident "." Ident
```

## Single vs multi — the first thing to get right

```aql
select User { id, email } filter .id = $id;      # ONE row: implicit LIMIT 1, single-row result type
multi select User { id, email };                 # ALL rows: no implicit limit, list result type
```

`limit` and `offset` are only accepted on a `multi select` (`limit/offset require 'multi select'`
otherwise).

## Shapes

```aql
multi select Post {
  title,
  author: { id, email },        # single link  → row_to_json subquery
  tags: { name }                # multi link   → json_agg subquery, '[]' when empty
} filter .author.role = Role.Admin order by .created_at desc limit 5;
```

Nested shapes compile to correlated subqueries in one statement — no N+1, no second round trip.
`rel-load-strategy: join` in `axel.yaml` switches them to `LEFT JOIN LATERAL`.

There are **no reverse links**. If `Post` has `link author: User`, then `User` has no `.posts`.
Query from `Post`, or correlate an aggregate (below).

## Filters, parameters, casts

```aql
multi select User { id }
  filter .active = true and (.role = Role.Admin or .age >= $min_age);

multi select User { id } filter .email ilike $pattern;
multi select User { id } filter .id in $ids;
multi select User { id } filter .created_at >= $since<datetime>;   # explicit cast
```

`$name` is a named parameter; the compiled SQL is positional with a comment header naming and
typing each one:

```sql
-- $1: min_age (int32)
```

Optional parameters (`$name?`) and `var` blocks declare a parameter's type up front — see
`aql/parameters/*` in the docs.

## Computed and aggregate fields

```aql
multi select User { email, upper_email := upper(.email) };

# Arithmetic in computed shape fields
multi select OrderItem {
  id,
  subtotal := .unit_price * .quantity,
  total := (.unit_price * .quantity) - .discount
};

select count(User filter .active = true);         # aggregate select: aggregates only, one row

# Aggregate shape with expressions (min/max/sum/avg/count with arbitrary expressions or math)
select Place {
  nearest := min(haversine(.loc.latitude, .loc.longitude, $target_lat, $target_lon))
};

# Correlated aggregate subquery as a scalar operand in filter or update
multi select User { id, email }
  filter (select count(Post filter .author.id = User.id)) > 0;
```

An aggregate select may not mix aggregate fields with plain row fields unless grouped with `group by`, and is never `multi`. Aggregate shape fields accept general expressions inside aggregate function calls.

## Group by

```aql
multi select User { role, n := count(.id) } group by .role having count(.id) > 5;
```

## Insert, update, delete

```aql
insert User { email := $email, role := Role.Member };

insert User { email := $email }
  unless conflict on .email
  else ( update User set { updated_at := $now } );      # ON CONFLICT … DO UPDATE

insert User { email := $email } unless conflict;        # ON CONFLICT DO NOTHING

update Post filter .id = $id set { title := $title };
update Post filter .id = $id set { author := (select User filter .email = $email).id };

# Multi-link updates: delta (+ and -) or full set replacement
update Organization filter .id = $id set {
  members := {
    "+": (multi select User filter .email in $emails),
    "-": (select User filter .id = $removed_user_id)
  }
};
update Organization filter .id = $id set {
  members := (multi select User filter .active = true)
};

delete Post filter .created_at < $cutoff;
```

## Bulk insert (`for` loop) & variable defaults

Iterate over multi-valued parameters or set literals to perform set-based bulk insertions:

```aql
var multi $conditions: str? := {'Hot', 'Cold', 'Fragile', 'Frozen'}

for $condition in $conditions {
  insert PackageCondition {
    name := $condition,
    added_by := (select User filter .email = 'ameenmohammed2311@gmail.com')
  } unless conflict;
}
```

- Declaring `multi $name: type? := default` passes an array parameter (`TEXT[]`) with a fallback default array.
- Emits a CTE `WITH __for_iter AS (SELECT unnest(...) AS "iter")` and runs `INSERT ... SELECT ... FROM __for_iter`.

Enum values are qualified (`Role.Member`). Insert and update return the row's columns.

## with blocks

```aql
with (
  admins := (multi select User { id, email } filter .role = Role.Admin)
)
multi select Post { title };
```

A binding compiles to a CTE. A single-row binding is a value; a `multi` binding is a set, and using
a set where a value is expected is an error.

## Errors you will see from `axel compile`

| Message | Meaning |
|---|---|
| `limit/offset require 'multi select'` | Add `multi` |
| `type "X" has no field "y"` | Typo, or a reverse link that does not exist |
| `an aggregate select returns a single row; drop 'multi'` | `count(...)` is already one row |
| `an aggregate select may only contain aggregate fields` | Split the query, or use a correlated subquery |
| `lexer: invalid input text "-- …"` | AQL comments are `#` |
| parameters vanish when run inline | The shell ate `$name`; single-quote the `--aql` value |
