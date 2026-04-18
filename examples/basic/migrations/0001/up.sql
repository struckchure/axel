CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE "user" (
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now(),
  "email" TEXT NOT NULL UNIQUE,
  "name" TEXT DEFAULT 'n/a',
  "age" INTEGER NOT NULL,
  "health" INTEGER NOT NULL,
  "active" BOOLEAN DEFAULT true,
  "id" UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE PRIMARY KEY
);

CREATE TABLE "post" (
  "content" TEXT NOT NULL,
  "id" UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE PRIMARY KEY,
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now(),
  "title" TEXT NOT NULL,
  "author" UUID NOT NULL,
  FOREIGN KEY ("author") REFERENCES "user"("id") ON DELETE CASCADE
);

CREATE TABLE "post_likes" (
  "post" UUID NOT NULL,
  "user" UUID NOT NULL,
  PRIMARY KEY ("post", "user"),
  FOREIGN KEY ("post") REFERENCES "post"(id) ON DELETE CASCADE,
  FOREIGN KEY ("user") REFERENCES "user"(id) ON DELETE CASCADE
);

CREATE TABLE "comment" (
  "content" TEXT NOT NULL,
  "id" UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE PRIMARY KEY,
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now(),
  "post" UUID NOT NULL,
  "author" UUID NOT NULL,
  FOREIGN KEY ("post") REFERENCES "post"("id") ON DELETE CASCADE,
  FOREIGN KEY ("author") REFERENCES "user"("id") ON DELETE CASCADE
);