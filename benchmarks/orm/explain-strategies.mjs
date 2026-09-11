import {readFileSync,writeFileSync} from 'node:fs';
import pg from 'pg';
import {url,assertBenchmarkDatabase} from './config.mjs';

assertBenchmarkDatabase();
const db=new pg.Client({connectionString:url});
const plans={};
await db.connect();
try {
  for(const adapter of ['axel-aql','axel-aql-join','axel-builder','axel-builder-join']) {
    const sample=JSON.parse(readFileSync(`results/verification-strategies/${adapter}-r0-c1.json`,'utf8')).samples.nested10x5.queries[0];
    plans[adapter]={sql:sample.sql,params:sample.params,explain:(await db.query(`EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) ${sample.sql}`,sample.params)).rows[0]['QUERY PLAN']};
  }
  writeFileSync('results/nested-strategies/plans.json',JSON.stringify(plans,null,2));
  console.log('Saved PostgreSQL plans for both strategies in both Axel execution modes.');
} finally {await db.end();}
