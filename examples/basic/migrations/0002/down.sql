DROP TRIGGER IF EXISTS "trg_user_rewrite_base" ON "user";

DROP TRIGGER IF EXISTS "trg_post_rewrite_post" ON "post";

DROP TRIGGER IF EXISTS "trg_post_rewrite_base" ON "post";

DROP TRIGGER IF EXISTS "trg_comment_rewrite_base" ON "comment";

DROP FUNCTION IF EXISTS "axel_rw_post_1"();

DROP FUNCTION IF EXISTS "axel_rw_base_1"();

ALTER TABLE "post" DROP COLUMN IF EXISTS slug;

DROP EXTENSION IF EXISTS "pgcrypto";