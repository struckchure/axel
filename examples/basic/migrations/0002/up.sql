CREATE EXTENSION IF NOT EXISTS "pgcrypto";

ALTER TABLE "post" ADD COLUMN slug TEXT;

CREATE OR REPLACE FUNCTION "axel_rw_base_1"() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'UPDATE' THEN
    NEW."updated_at" := now();
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION "axel_rw_post_1"() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    NEW."slug" := NEW."content";
  END IF;
  IF TG_OP = 'UPDATE' THEN
    NEW."slug" := NEW."content";
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER "trg_comment_rewrite_base" BEFORE UPDATE ON "comment"
  FOR EACH ROW EXECUTE FUNCTION "axel_rw_base_1"();

CREATE TRIGGER "trg_post_rewrite_base" BEFORE UPDATE ON "post"
  FOR EACH ROW EXECUTE FUNCTION "axel_rw_base_1"();

CREATE TRIGGER "trg_post_rewrite_post" BEFORE INSERT OR UPDATE ON "post"
  FOR EACH ROW EXECUTE FUNCTION "axel_rw_post_1"();

CREATE TRIGGER "trg_user_rewrite_base" BEFORE UPDATE ON "user"
  FOR EACH ROW EXECUTE FUNCTION "axel_rw_base_1"();