```sh
$ axel -d ./examples/basic compile --aql 'select Post { id, title, author: { id } } filter .id = $id;'
-- $1: id
SELECT
  p.id AS id,
  p.title AS title,
  (SELECT row_to_json(u_author_sub) FROM (SELECT u_author.id AS id FROM "user" u_author WHERE u_author.id = p.author_id LIMIT 1) u_author_sub) AS author
FROM "post" p
WHERE p.id = $1;
```

```sh
$ axel -d ./examples/basic compile -f examples/basic/list_post.query.aql -o examples/basic/list_post.query.sql
written to examples/basic/list_post.query.sql
```

```sh
$ axel -d ./examples/basic codegen -q examples/basic/list_post.query.aql -o ./examples/basic/gen -g go
codegen complete → ./examples/basic/gen
```
