---
title: Links — ASL
description: Single and multi foreign-key relationships
---

# Links

Links define foreign-key relationships between types.

## Single link (FK column)

```asl
type Post {
  required link author: User;    # adds author_id FK column
}
```

## Multi link (junction table)

```asl
type Post {
  multi link tags: Tag;          # creates post_tags junction table
}
```

The junction table name is `{source}_{link}` in snake_case (e.g. `post_tags`).

## Required links

```asl
type Comment {
  required link post: Post;      # post_id NOT NULL
  required link author: User;    # author_id NOT NULL
}
```
