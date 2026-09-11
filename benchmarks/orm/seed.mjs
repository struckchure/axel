import pg from 'pg';
import { url, assertBenchmarkDatabase } from './config.mjs';
assertBenchmarkDatabase();
const db = new pg.Client({connectionString:url});
await db.connect();
try {
  await db.query(`
    DROP TABLE IF EXISTS post;
    DROP TABLE IF EXISTS account;
    CREATE TABLE account (id integer PRIMARY KEY, email text NOT NULL, age integer NOT NULL, active boolean NOT NULL);
    CREATE TABLE post (id integer PRIMARY KEY, title text NOT NULL, author integer NOT NULL REFERENCES account(id));
    CREATE INDEX post_author_idx ON post(author);
    INSERT INTO account SELECT i, 'user' || i || '@example.test', 18 + i % 63, i % 2 = 0 FROM generate_series(1,10000) i;
    INSERT INTO post SELECT i, 'Post ' || i, (i-1)/5+1 FROM generate_series(1,50000) i;
  `);
  await db.query('VACUUM ANALYZE account');
  await db.query('VACUUM ANALYZE post');
  console.log('Seeded 10,000 accounts and 50,000 posts.', (await db.query('select version()')).rows[0]);
} finally {await db.end();}
