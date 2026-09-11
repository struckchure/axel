import {execFileSync} from 'node:child_process';
import {mkdirSync,writeFileSync} from 'node:fs';
import os from 'node:os';
import pg from 'pg';
import {url,assertBenchmarkDatabase} from './config.mjs';
import {metadata} from './metadata.mjs';
assertBenchmarkDatabase();
const adapters=(process.env.BENCH_ADAPTERS??'axel-aql,axel-aql-join,axel-builder,axel-builder-join,prisma6,prisma7,typeorm,drizzle').split(',');
const rounds=Number(process.env.BENCH_ROUNDS??8);
const concurrencies=(process.env.BENCH_CONCURRENCIES??'1,10').split(',').map(Number);
const out=process.env.BENCH_OUTPUT??`results/${new Date().toISOString().replaceAll(':','-')}`;
mkdirSync(out,{recursive:true});
const db=new pg.Client({connectionString:url});await db.connect();
const postgres=(await db.query('select version()')).rows[0].version;
await db.end();
const details=await metadata();
const runtimePatch=execFileSync('git',['diff','--','../../generators/typescript/generator.go'],{encoding:'utf8'});
if(runtimePatch) writeFileSync(`${out}/runtime.patch`,runtimePatch);
writeFileSync(`${out}/environment.json`,JSON.stringify({date:new Date().toISOString(),node:process.version,platform:process.platform,arch:process.arch,cpu:os.cpus()[0].model,cpuCount:os.cpus().length,memoryBytes:os.totalmem(),postgres,axelCommit:execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),adapters,rounds,concurrencies,workloads:(process.env.BENCH_WORKLOADS??'point,range100,nested10x5,insert').split(','),iterations:Number(process.env.BENCH_ITERATIONS??1000),warmup:Number(process.env.BENCH_WARMUP??100),...details,...(runtimePatch?{measurementNote:'This run uses the working-tree TypeScript runtime strategy implementation. See runtime.patch; unsuffixed Axel entries use correlated aggregates and -join entries use lateral aggregates.'}:{})},null,2));
for(let round=0;round<rounds;round++) {
  // Cyclic rotation balances position; alternate concurrency order each round.
  const order=[...adapters.slice(round%adapters.length),...adapters.slice(0,round%adapters.length)];
  for(const concurrency of round%2?[...concurrencies].reverse():concurrencies) {
    for(const adapter of order) execFileSync(process.execPath,['--import','tsx','worker.ts',adapter,String(round),String(concurrency),`${out}/${adapter}-r${round}-c${concurrency}.json`],{stdio:'inherit',env:process.env});
  }
}
console.log(`Results: ${out}`);
