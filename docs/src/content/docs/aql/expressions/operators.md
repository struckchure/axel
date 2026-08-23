---
title: "Operators — AQL"
description: "Comparison and logical operators, and combining conditions"
---

# Operators

| AQL operator | SQL equivalent |
| ------------ | -------------- |
| `+`          | `+` (addition or unary plus) |
| `-`          | `-` (subtraction or unary minus) |
| `*`          | `*` (multiplication) |
| `/`          | `/` (division) |
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
| `is null`      | `IS NULL`      |
| `is not null`  | `IS NOT NULL`  |

## Arithmetic

AQL supports binary arithmetic operators (`+`, `-`, `*`, `/`) and unary signs (`+`, `-`). Expressions follow standard mathematical operator precedence:

1. **Unary** `+`, `-` (e.g. `- .discount`)
2. **Multiplicative** `*`, `/`
3. **Additive** `+`, `-`
4. **Comparisons & Null tests** `=`, `!=`, `<`, `<=`, `>`, `>=`, `is [not] null`, `??`
5. **Logical** `and`, then `or`

```aql
# Computed shape fields
select Order {
  id,
  subtotal,
  tax := .subtotal * 0.2,
  total := (.subtotal * 1.2) - .discount
};

# Filtering with arithmetic
multi select Product { id, title }
  filter .quantity * .unit_price >= $min_total - $rebate;

# Updating with arithmetic
update Account filter .id = $id<uuid>
  set { balance := .balance - $amount<int64> };

# Ordering by calculated expressions
multi select Product { id, name }
  order by .unit_price * .stock_count desc;
```


## Null tests

`is null` / `is not null` are postfix operators — they test the operand on their
left and take no right-hand side:

```aql
multi select Doc { id } filter .deleted_at is null;
multi select Doc { id } filter .published_at is not null;
```

This is distinct from `??` (coalesce), which substitutes a fallback value rather
than testing for presence.

## Combining conditions

Conditions chain with `and` / `or` to any length. As in SQL, **`and` binds tighter than `or`**, so
`a or b and c` means `a or (b and c)`.

```aql
multi select Project { *, members: { id } }
filter .owner = $owner<str> and .organization = $organization<str>
order by .created_at desc;
```

Parenthesize to group conditions explicitly. Groups nest to any depth:

```aql
multi select Post { id, title }
filter (.title like $q<str> or .content like $q<str>)
   and (.published = true or .author = $viewer<uuid>)
   and .deleted = false;
```

An [optional parameter](/aql/parameters/optional) inside a chain relaxes **only its own condition** —
the rest of the filter still applies. Here, omitting `$author` widens the search to every author,
but never returns an unpublished post:

```aql
multi select Post { id } filter .published = true and .author = $author<uuid>?;
```
