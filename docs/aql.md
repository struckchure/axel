# Axel Query Language (AQL)

AQL is a query language that compiles to parameterized PostgreSQL SQL. You write queries; Axel outputs SQL strings you execute however you like. Axel never runs queries for you.

---

## File extension

```
query.aql
```

---

## Output format

Every compiled query produces:

- A positional-parameter SQL string (`$1`, `$2`, ...)
- A comment header mapping parameter names to positions

```sql
-- $1: email (str)
-- $2: active (bool)
SELECT u.id AS id, u.email AS email
FROM "user" u
WHERE u.email = $1 AND u.active = $2;
```

The parameter order matches first-appearance order in the query.

---

## Parameters

Named parameters use a `$` prefix. They are collected in order of first appearance and mapped to positional `$N` SQL parameters.

```aql
select User filter .email = $email and .active = $active;
```

```sql
-- $1: email
-- $2: active
SELECT ...
FROM "user" u
WHERE u.email = $1 AND u.active = $2;
```

---

## SELECT

### Basic select

```aql
select User;
```

Selects all scalar properties of the type.

### Shape

A shape limits which fields are returned.

```aql
select User {
  id,
  email,
  name
};
```

```sql
SELECT u.id AS id, u.email AS email, u.name AS name
FROM "user" u;
```

### Filter

```aql
select User { id, email }
filter .active = true and .age >= $min_age;
```

```sql
-- $1: min_age
SELECT u.id AS id, u.email AS email
FROM "user" u
WHERE u.active = true AND u.age >= $1;
```

### Order by

```aql
select User { id, email }
order by .created_at desc;
```

```sql
SELECT u.id AS id, u.email AS email
FROM "user" u
ORDER BY u.created_at DESC;
```

Multiple fields:

```aql
select User { id, email }
order by .active desc, .created_at asc;
```

### Limit and offset

```aql
select User { id, email }
order by .created_at desc
limit $limit
offset $offset;
```

```sql
-- $1: limit
-- $2: offset
SELECT u.id AS id, u.email AS email
FROM "user" u
ORDER BY u.created_at DESC
LIMIT $1
OFFSET $2;
```

### Combining clauses

```aql
select User { id, email, name }
filter .active = true and .age >= $min_age
order by .created_at desc
limit $limit
offset $offset;
```

---

## Nested shapes (links)

Shapes can include linked types. Axel compiles nested shapes into a single SQL query using `row_to_json` or `json_agg` — no N+1.

### Single link

Returns a JSON object.

```aql
select Post {
  id,
  title,
  author: {
    id,
    email
  }
};
```

```sql
SELECT
  p.id AS id,
  p.title AS title,
  (SELECT row_to_json(u_author_sub)
   FROM (
     SELECT u_author.id AS id, u_author.email AS email
     FROM "user" u_author
     WHERE u_author.id = p.author_id
     LIMIT 1
   ) u_author_sub) AS author
FROM "post" p;
```

### Multi link

Returns a JSON array. Empty results return `[]` rather than `null`.

```aql
select Post {
  id,
  title,
  likes: {
    id,
    email
  }
};
```

```sql
SELECT
  p.id AS id,
  p.title AS title,
  (SELECT COALESCE(json_agg(row_to_json(u_likes_sub)), '[]')
   FROM (
     SELECT u_likes.id AS id, u_likes.email AS email
     FROM "post_likes" jt_likes
     JOIN "user" u_likes ON u_likes.id = jt_likes.user_id
     WHERE jt_likes.post_id = p.id
   ) u_likes_sub) AS likes
FROM "post" p;
```

---

## Aggregate SELECT

### count

```aql
select count(User);
```

```sql
SELECT COUNT(*) FROM (
  SELECT 1 FROM "user" u
) _agg;
```

With a filter:

```aql
select count(User filter .active = true);
```

```sql
SELECT COUNT(*) FROM (
  SELECT 1 FROM "user" u
  WHERE u.active = true
) _agg;
```

---

## INSERT

```aql
insert User {
  email := $email,
  name  := $name,
  age   := $age
};
```

```sql
-- $1: email
-- $2: name
-- $3: age
INSERT INTO "user" ("email", "name", "age")
VALUES ($1, $2, $3)
RETURNING *;
```

### Inserting with a link

Assign a link by passing a subquery that resolves to the FK value.

```aql
insert Post {
  title  := $title,
  author := (select User filter .email = $email)
};
```

```sql
-- $1: title
-- $2: email
INSERT INTO "post" ("title", "author_id")
VALUES ($1, (SELECT u.id FROM "user" u WHERE u.email = $2 LIMIT 1))
RETURNING *;
```

---

## UPDATE

```aql
update User
filter .id = $id
set {
  name   := $name,
  active := $active
};
```

```sql
-- $1: name
-- $2: active
-- $3: id
UPDATE "user" u SET
  name = $1,
  active = $2
WHERE u.id = $3
RETURNING *;
```

---

## DELETE

```aql
delete User filter .id = $id;
```

```sql
-- $1: id
DELETE FROM "user" u
WHERE u.id = $1;
```

---

## Operators

| AQL operator | SQL equivalent |
| ------------ | -------------- |
| `=`          | `=`            |
| `!=`         | `!=`           |
| `<`          | `<`            |
| `<=`         | `<=`           |
| `>`          | `>`            |
| `>=`         | `>=`           |
| `and`        | `AND`          |
| `or`         | `OR`           |
| `??`         | `COALESCE`     |
| `in`         | `IN`           |
| `like`       | `LIKE`         |
| `ilike`      | `ILIKE`        |

---

## Literals

| Value   | Example         |
| ------- | --------------- |
| String  | `'hello'`       |
| Integer | `42`            |
| Float   | `3.14`          |
| Boolean | `true`, `false` |
| Null    | `null`          |

---

## Path expressions

Paths starting with `.` refer to fields on the current type. The compiler resolves them to `alias.column`.

```aql
filter .active = true and .age >= $min_age
order by .created_at desc
```

Multi-step paths traverse a link:

```aql
filter .author.email = $email
```

---

## Grammar reference

```
Statement   = SelectStmt | InsertStmt | UpdateStmt | DeleteStmt

SelectStmt  = "select" SelectBody ";"
SelectBody  = AggExpr
            | TypeName Shape? Filter? OrderBy? Limit? Offset?

AggExpr     = Ident "(" TypeName Filter? ")"

InsertStmt  = "insert" TypeName "{" Assignment ("," Assignment)* ","? "}" ";"
UpdateStmt  = "update" TypeName Filter? "set" "{" Assignment ("," Assignment)* ","? "}" ";"
DeleteStmt  = "delete" TypeName Filter? ";"

Shape       = "{" ShapeField ("," ShapeField)* ","? "}"
ShapeField  = Ident (":" Shape)?

Assignment  = Ident ":=" Expr

Filter      = "filter" Expr
OrderBy     = "order" "by" OrderItem ("," OrderItem)*
OrderItem   = Expr ("asc" | "desc")?
Limit       = "limit" Expr
Offset      = "offset" Expr

Expr        = Primary (BinOp Primary)?
BinOp       = "=" | "!=" | "<" | "<=" | ">" | ">=" | "and" | "or" | "??" | "in" | "like" | "ilike"

Primary     = "(" Expr ")"
            | FuncCall
            | PathExpr
            | "$" Ident
            | "null" | "true" | "false"
            | String | Int | Float | Ident

FuncCall    = Ident "(" (Expr ("," Expr)*)? ")"
PathExpr    = ("." Ident)+
```
