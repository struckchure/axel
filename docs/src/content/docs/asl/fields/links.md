---
title: "Links — ASL"
description: "Single and multi foreign-key relationships"
---

# Links

Links define foreign-key relationships between types.

## Single link (FK column)

```asl
type Post {
  required link author: User;    # adds an FK column named "author"
}
```

The column is named after the **field**, not the target type, and carries no `_id` suffix — `link
author: User` gives you `post.author`, referencing `user.id`. Filters and shapes still use the field
name (`.author`, `author: { … }`), so the column name only matters when you read the generated SQL.

## Multi link (junction table)

```asl
type Post {
  multi link tags: Tag;          # creates post_tags junction table
}
```

The junction table name is `{source}_{link}` in snake_case (e.g. `post_tags`), and its two FK
columns are named after the tables they reference:

```sql
CREATE TABLE "post_tags" (
  "post" UUID NOT NULL,
  "tag" UUID NOT NULL,
  CONSTRAINT "pk_post_tags" PRIMARY KEY ("post", "tag"),
  CONSTRAINT "fk_post_tags_post" FOREIGN KEY ("post") REFERENCES "post"("id") ON DELETE CASCADE,
  CONSTRAINT "fk_post_tags_tag" FOREIGN KEY ("tag") REFERENCES "tag"("id") ON DELETE CASCADE
);
```

### Self-referential multi links

When a multi link points back at its own type, naming both sides after the referenced table would
produce two columns called `product`, which Postgres rejects (`column "product" appears twice in
primary key constraint`). The target side falls back to the **link name**:

```asl
type Product {
  required id: uuid { constraint pk; };
  multi link addons: Product;    # product_addons("product", "addons")
}
```

Nothing about querying changes — `select Product { id, addons: { id } }` works as it does for any
other multi link.

## Multi scalars are not links

`multi` on a scalar field means something different from `multi link`: it declares an **array
column** on the row, not a junction table.

```asl
type User {
  multi link teams: Team;   # junction table "user_teams"
  multi roles: UserType;    # a single TEXT[] column "roles"
}
```

The distinction shows up in two places:

- **Membership.** `in` against a multi scalar compiles to `= ANY(...)`, because Postgres `IN` takes a
  parenthesised list rather than an array. Against a multi link it compiles to an `EXISTS` over the
  junction table.

  ```aql
  select User { id } filter UserType.Admin in .roles;   # → 'Admin' = ANY(u.roles)
  select User { id } filter $team<uuid> in .teams;      # → EXISTS (SELECT 1 FROM "user_teams" …)
  ```

- **Assignment.** Delta assignment (`{ "+": …, "-": … }`) applies only to a multi link. A multi scalar
  is assigned as a whole array — see [Updating links](/aql/update/links).

## Required links

```asl
type Comment {
  required link post: Post;      # column "post", NOT NULL
  required link author: User;    # column "author", NOT NULL
}
```

Adding a **required** link to a table that already has rows needs a backfill, exactly like adding a
required column — see [`axel diff`](/cli#axel-diff).
