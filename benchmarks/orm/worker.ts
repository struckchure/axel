import 'reflect-metadata';
import pg from 'pg';
import assert from 'node:assert/strict';
import { performance } from 'node:perf_hooks';
import { writeFileSync } from 'node:fs';
import { url, assertBenchmarkDatabase } from './config.mjs';

const [adapter, roundArg, concurrencyArg, output] = process.argv.slice(2);
const concurrency = Number(concurrencyArg);
const round = Number(roundArg);
const warmup = Number(process.env.BENCH_WARMUP ?? 100);
const iterations = Number(process.env.BENCH_ITERATIONS ?? 1000);
const captureSQL = process.env.BENCH_CAPTURE === '1';
assertBenchmarkDatabase();
type Row = Record<string, any>;
type Operation = (i: number) => Promise<Row[]>;
const pool = new pg.Pool({connectionString:url, max:concurrency});
const fields = {id:true, email:true, age:true, active:true};
const start = (i:number) => 1 + (i * 97 % 9900);
const data = (i:number) => ({id:10001+i, email:`insert${i}@example.test`, age:30, active:true});
const raw:Record<string,Operation> = {
  point: async i => (await pool.query('SELECT id, email, age, active FROM account WHERE id = $1', [start(i)])).rows,
  range100: async i => (await pool.query('SELECT id, email, age, active FROM account WHERE id >= $1 AND id < $2', [start(i),start(i)+100])).rows,
  nested10x5: async i => (await pool.query(`SELECT u.id, u.email, (SELECT COALESCE(json_agg(row_to_json(p)), '[]') FROM (SELECT p.id, p.title FROM post p WHERE p.author = u.id) p) AS posts FROM account u WHERE u.id >= $1 AND u.id < $2`, [start(i),start(i)+10])).rows,
};
let ops:Record<string,Operation>;
let close = async () => {};
let captured:unknown[] = [];
let capture = true;
const origQuery = pool.query.bind(pool);
const captureQuery = (...args:any[]) => {
  if(capture) captured.push({sql:typeof args[0]==='string'?args[0]:args[0].text, params:args[1]});
  return (origQuery as any)(...args);
};

if(['axel-aql','axel-builder','axel-aql-join','axel-builder-join'].includes(adapter)) {
  const { Runner } = await import('./generated/axel/runner.ts');
  const db = new Runner(pool, {relLoadStrategy:adapter.endsWith('-join')?'join':'query'});
  ops = adapter.startsWith('axel-aql') ? {
    point: i => db.run('select Account { id, email, age, active } filter .id = $id;', {id:start(i)}),
    range100: i => db.run('select Account { id, email, age, active } filter .id >= $lo and .id < $hi;', {lo:start(i),hi:start(i)+100}),
    nested10x5: i => db.run('select Account { id, email, posts := (select Post { id, title } filter .author = Account.id) } filter .id >= $lo and .id < $hi;', {lo:start(i),hi:start(i)+10}),
    insert: i => db.run('insert Account { id := $id, email := $email, age := $age, active := $active };', data(i)),
  } : {
    point: i => db.select('Account',fields).where('id','=',start(i)).all(),
    range100: i => db.select('Account',fields).where('id','>=',start(i)).and('id','<',start(i)+100).all(),
    nested10x5: i => db.select('Account',{id:true,email:true,posts:db.select('Post',{id:true,title:true}).where('author','=', '`Account.id`')}).where('id','>=',start(i)).and('id','<',start(i)+10).all(),
    insert: i => db.insert('Account',data(i)).all(),
  };
} else if(adapter === 'prisma6' || adapter === 'prisma7' || adapter === 'prisma7-query') {
  const major = adapter === 'prisma6' ? 6 : 7;
  const {PrismaClient} = await import(major === 6 ? './prisma6/generated/index.js' : './prisma7/generated/client.ts');
  let options:any = {log:captureSQL?[{emit:'event',level:'query'}]:[]};
  if(major === 6) options.datasources = {db:{url:`${url}?connection_limit=${concurrency}&pool_timeout=0`}};
  else {
    const {PrismaPg} = await import('./prisma7/node_modules/@prisma/adapter-pg/dist/index.mjs');
    options.adapter = new PrismaPg({connectionString:url,max:concurrency});
  }
  const db = new PrismaClient(options);
  db.$on('query',(e:any) => {if(capture) captured.push({sql:e.query,params:e.params});});
  await db.$connect();
  const relation = {relationLoadStrategy:adapter === 'prisma7-query'?'query':'join'};
  ops = {
    point: async i => [await db.account.findUniqueOrThrow({where:{id:start(i)},select:fields})],
    range100: i => db.account.findMany({where:{id:{gte:start(i),lt:start(i)+100}},select:fields}),
    nested10x5: i => db.account.findMany({...relation,where:{id:{gte:start(i),lt:start(i)+10}},select:{id:true,email:true,posts:{select:{id:true,title:true}}}}),
    insert: async i => [await db.account.create({data:data(i),select:fields})],
  };
  close=()=>db.$disconnect();
} else if(adapter === 'typeorm') {
  const {DataSource,EntitySchema,And,MoreThanOrEqual,LessThan} = await import('typeorm');
  const user = new EntitySchema({name:'Account',tableName:'account',columns:{id:{type:Number,primary:true},email:{type:String},age:{type:Number},active:{type:Boolean}},relations:{posts:{type:'one-to-many',target:'Post',inverseSide:'user'}}});
  const post = new EntitySchema({name:'Post',tableName:'post',columns:{id:{type:Number,primary:true},title:{type:String},author:{type:Number}},relations:{user:{type:'many-to-one',target:'Account',joinColumn:{name:'author'},inverseSide:'posts'}}});
  const logger:any={logQuery:(sql:string,params:any)=>{if(capture)captured.push({sql,params});},logQueryError:()=>{},logQuerySlow:()=>{},logSchemaBuild:()=>{},logMigration:()=>{},log:()=>{}};
  const db = new DataSource({type:'postgres',url,entities:[user,post],synchronize:false,extra:{max:concurrency},logging:captureSQL?['query']:false,logger});
  await db.initialize();
  const repo = db.getRepository('Account');
  ops={
    point: async i => [await repo.findOneByOrFail({id:start(i)})],
    range100: i => repo.find({where:{id:And(MoreThanOrEqual(start(i)),LessThan(start(i)+100))},select:fields}),
    nested10x5: i => repo.find({where:{id:And(MoreThanOrEqual(start(i)),LessThan(start(i)+10))},relations:{posts:true},relationLoadStrategy:'join',select:{id:true,email:true,posts:{id:true,title:true}}}),
    insert: async i => (await repo.createQueryBuilder().insert().values(data(i)).returning(['id','email','age','active']).execute()).raw,
  };
  close=()=>db.destroy();
} else if(adapter === 'drizzle') {
  const {drizzle} = await import('drizzle-orm/node-postgres');
  const {pgTable,integer,text,boolean} = await import('drizzle-orm/pg-core');
  const {eq,gte,lt,and,relations} = await import('drizzle-orm');
  const users = pgTable('account',{id:integer('id').primaryKey(),email:text('email').notNull(),age:integer('age').notNull(),active:boolean('active').notNull()});
  const posts = pgTable('post',{id:integer('id').primaryKey(),title:text('title').notNull(),author:integer('author').notNull().references(()=>users.id)});
  const usersRelations=relations(users,({many})=>({posts:many(posts)}));
  const postsRelations=relations(posts,({one})=>({user:one(users,{fields:[posts.author],references:[users.id]})}));
  const db=drizzle(pool,{schema:{users,posts,usersRelations,postsRelations}});
  ops={
    point:i=>db.select().from(users).where(eq(users.id,start(i))),
    range100:i=>db.select().from(users).where(and(gte(users.id,start(i)),lt(users.id,start(i)+100))),
    nested10x5:i=>db.query.users.findMany({columns:{id:true,email:true},where:and(gte(users.id,start(i)),lt(users.id,start(i)+10)),with:{posts:{columns:{id:true,title:true}}}}),
    insert:i=>db.insert(users).values(data(i)).returning(),
  };
} else throw new Error(`Unknown adapter ${adapter}`);

// Canonicalization is only used during verification, outside timing.
function canonical(value:any):any {
  if(Array.isArray(value)) return value.map(canonical).sort((a,b)=>a.id-b.id);
  if(value && typeof value==='object') return Object.fromEntries(Object.keys(value).sort().map(k=>[k,canonical(value[k])]));
  return value;
}
const samples:Record<string,unknown>={};
const results=[];
try {
  for(const [workload,op] of Object.entries(ops!)) {
    if(process.env.BENCH_WORKLOADS && !process.env.BENCH_WORKLOADS.split(',').includes(workload)) continue;
    await pool.query('DELETE FROM account WHERE id > 10000');
    capture=false;
    const expected=workload==='insert'?[data(0)]:await raw[workload](0);
    (pool as any).query = captureQuery;
    captured=[];capture=true;
    const actual=await op(0);
    capture=false;
    assert.deepEqual(canonical(actual),canonical(expected),`${adapter}/${workload} differs from raw pg`);
    samples[workload]={queries:captured,resultRows:actual.length};
    if(adapter.startsWith('axel-') && workload === 'nested10x5') {
      assert.equal(captured.length,1);
      assert.equal((captured[0] as any).sql.includes('LEFT JOIN LATERAL'),adapter.endsWith('-join'),'Relation strategy must change the emitted SQL');
      assert.match((captured[0] as any).sql,/json_agg/);
    }
    if(adapter === 'prisma7-query' && workload === 'nested10x5' && captureSQL) {
      assert.equal(captured.length,2,'Prisma query strategy must issue one parent and one child query');
      assert.ok(captured.every((q:any)=>!q.sql.includes('LATERAL')));
    }
    (pool as any).query = origQuery;
    if(workload==='insert') {
      assert.deepEqual(canonical((await pool.query('SELECT id,email,age,active FROM account WHERE id=10001')).rows),canonical(expected));
    } else {
      for(const i of [7,101]) assert.deepEqual(canonical(await op(i)),canonical(await raw[workload](i)),`${adapter}/${workload} case ${i}`);
    }
    await pool.query('DELETE FROM account WHERE id > 10000');
    for(let i=0;i<warmup;i++) await op(i);
    await pool.query('DELETE FROM account WHERE id > 10000');
    // Establish every pool connection before measurement.
    await Promise.all(Array.from({length:concurrency},(_,i)=>ops!.point(i)));
    let next=0;
    const times:number[]=[];
    const t0=performance.now();
    await Promise.all(Array.from({length:concurrency},async()=>{
      while(next<iterations) {
        const i=next++;
        const t=performance.now();
        const rows=await op(i);
        times.push(performance.now()-t);
        assert.equal(rows.length,workload==='range100'?100:workload==='nested10x5'?10:1);
      }
    }));
    const elapsed=performance.now()-t0;
    times.sort((a,b)=>a-b);
    const pct=(p:number)=>times[Math.ceil(times.length*p)-1];
    results.push({adapter,round,concurrency,workload,iterations,warmup,elapsedMs:elapsed,opsPerSec:iterations*1000/elapsed,p50Ms:pct(.5),p95Ms:pct(.95),p99Ms:pct(.99),latenciesMs:times});
    await pool.query('DELETE FROM account WHERE id > 10000');
  }
  writeFileSync(output,JSON.stringify({results,samples},null,2));
  console.log(`${adapter} round=${round} concurrency=${concurrency} verified + measured`);
} finally {await close();await pool.end();}
