SELECT
  p.id AS id,
  p.title AS title,
  (SELECT row_to_json(u_author_sub) FROM (SELECT u_author.id AS id, u_author.name AS name FROM "user" u_author WHERE u_author.id = p.author_id LIMIT 1) u_author_sub) AS author
FROM "post" p;
