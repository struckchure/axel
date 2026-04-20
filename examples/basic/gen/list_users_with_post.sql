SELECT
  u.id AS id,
  u.email AS email,
  (SELECT json_agg(row_to_json(p_posts_sub)) FROM (SELECT p.id AS id, p.title AS title FROM "post" p WHERE p.author = u.id) p_posts_sub) AS posts
FROM "user" u;
