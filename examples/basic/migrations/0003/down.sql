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

DROP FUNCTION IF EXISTS "slugify"(text);

DROP EXTENSION IF EXISTS "unaccent";

DROP EXTENSION IF EXISTS "pgcrypto";