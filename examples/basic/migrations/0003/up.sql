CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE EXTENSION IF NOT EXISTS "unaccent";

CREATE OR REPLACE FUNCTION "slugify"(value text) RETURNS text AS $$
BEGIN
  RETURN regexp_replace( regexp_replace(lower(public.unaccent(value)), '[^a-z0-9\-_]+', '-', 'gi'), '(^-+|-+$)', '', 'g' );
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION "axel_rw_post_1"() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    NEW."slug" := slugify(NEW."title");
  END IF;
  IF TG_OP = 'UPDATE' THEN
    NEW."slug" := slugify(NEW."title");
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;