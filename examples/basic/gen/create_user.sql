-- $1: email (str)
BEGIN;
INSERT INTO "user" ("email", "age", "health")
VALUES ($1, 100, 100)
RETURNING *;
COMMIT;
