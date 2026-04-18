```sh
$ axel --dir ./examples/basic compile --aql 'select Post { id, title, author: { id } } filter .id = $id;'
-- $1: id
SELECT
  p.id AS id,
  p.title AS title,
  (SELECT row_to_json(u_author_sub) FROM (SELECT u_author.id AS id FROM "user" u_author WHERE u_author.id = p.author_id LIMIT 1) u_author_sub) AS author
FROM "post" p
WHERE p.id = $1;
```
