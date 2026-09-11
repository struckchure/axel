import assert from 'node:assert/strict';
import pg from 'pg';
import {Runner} from './generated/axel/runner.ts';
import {url,assertBenchmarkDatabase} from './config.mjs';

assertBenchmarkDatabase();
const db=new pg.Pool({connectionString:url,max:1});
const aggregate=new Runner(db);
const join=new Runner(db,{relLoadStrategy:'join'});
try {
  // Empty child selections must not drop the parent or change null semantics.
  await db.query("INSERT INTO account VALUES(10001,'empty@example.test',30,true)");
  for(const r of [aggregate,join]) {
    const aql=await r.run('select Account { id, posts := (select Post { id, title } filter .author = Account.id) } filter .id = $id',{id:10001});
    const builder=await r.select('Account',{id:true,posts:r.select('Post',{id:true,title:true}).where('author','=', '`Account.id`')}).where('id','=',10001).all();
    assert.deepEqual(aql,[{id:10001,posts:null}]);
    assert.deepEqual(builder,aql);
  }
  // Two independent relations and an inner bound parameter exercise join aliases
  // and AQL parameter ordering against PostgreSQL, outside benchmark timing.
  const query='select Account { id, posts := (select Post { id } filter .author = Account.id and .id >= $min), all_posts := (select Post { id } filter .author = Account.id) } filter .id = $id';
  const expected=[{id:1,posts:[{id:3},{id:4},{id:5}],all_posts:[{id:1},{id:2},{id:3},{id:4},{id:5}]}];
  for(const r of [aggregate,join]) {
    const rows=await r.run(query,{min:3,id:1});
    for(const row of rows as any[]) for(const field of ['posts','all_posts']) row[field].sort((a:any,b:any)=>a.id-b.id);
    assert.deepEqual(rows,expected);
  }
  console.log('Both strategies preserve empty parents, nested data, independent relations, and bound parameters.');
} finally {
  await db.query('DELETE FROM account WHERE id=10001');
  await db.end();
}
