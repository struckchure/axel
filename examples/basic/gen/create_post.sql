-- $1: title (str)
-- $2: content (str)
BEGIN;
INSERT INTO "post" ("title", "content", "author")
VALUES ($1, $2, (SELECT u.id FROM "user" u WHERE u.email = 'user@mail.com' LIMIT 1))
RETURNING *;
COMMIT;
