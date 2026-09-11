import {readFileSync,readdirSync,writeFileSync,existsSync} from 'node:fs';
import {resolve,relative} from 'node:path';
const dir=process.argv[2];
if(!dir) throw new Error('Usage: pnpm report results/<run>');
const root=resolve(dir);
const order=['point','range100','nested10x5','insert'];
const names={'axel-aql':'Axel AQL','axel-aql-join':'Axel AQL (join)','axel-builder':'Axel builder','axel-builder-join':'Axel builder (join)',prisma6:'Prisma 6',prisma7:'Prisma 7','prisma7-query':'Prisma 7 (query)',typeorm:'TypeORM',drizzle:'Drizzle'};
const titles={point:'Point reads',range100:'100-row reads',nested10x5:'Nested relations',insert:'Inserts'};
const descriptions={point:'Read one account by primary key, returning id, email, age, and active.',range100:'Read 100 accounts in a primary-key range, returning the same four fields.',nested10x5:'Read 10 accounts with five posts each, returning account id/email and post id/title.',insert:'Insert one account and return all four fields.'};
const median=values=>{const a=[...values].sort((a,b)=>a-b);return a.length%2?a[(a.length-1)/2]:(a[a.length/2-1]+a[a.length/2])/2;};
function load(path) {
  return {path,env:JSON.parse(readFileSync(`${path}/environment.json`,'utf8')),results:readdirSync(path).filter(f=>/-r\d+-c\d+\.json$/.test(f)).flatMap(f=>JSON.parse(readFileSync(`${path}/${f}`,'utf8')).results)};
}
const primary=load(root);
const sourceFor=new Map((primary.env.workloads??order).map(w=>[w,primary]));
const additions=[];
const manifestPath=`${root}/report-sources.json`;
if(existsSync(manifestPath)) {
  const manifest=JSON.parse(readFileSync(manifestPath,'utf8'));
  for(const entry of manifest.supplements??[]) {
    const source=load(resolve(root,entry.path));
    for(const workload of entry.workloads) {
      if(sourceFor.has(workload)) throw new Error(`Duplicate report source for ${workload}`);
      sourceFor.set(workload,source);
    }
  }
}
if(existsSync(manifestPath)) {
  for(const entry of JSON.parse(readFileSync(manifestPath,'utf8')).additions??[]) {
    additions.push({...entry,source:load(resolve(root,entry.path))});
  }
}
const workloads=order.filter(w=>sourceFor.has(w));
const summary=[];
let md='# Axel ORM benchmark\n\n';
md+='Results are grouped by workload, with separate concurrency-1 and concurrency-10 tables. Every table is sorted by median throughput, highest first. **ops/sec means completed operations per second.**\n\n';
if(new Set(sourceFor.values()).size>1) md+='This report combines saved runs: point reads, 100-row reads, and inserts come from the original full-workload run; nested relations come from the newer aggregate-versus-join run. No measurements were rerun or pooled across runs. Each section identifies its source and implementation.\n\n';
md+='All runs use pnpm, Node 24.21.0, PostgreSQL 18.6, and the same 10,000-account / 50,000-post fixture on an Apple M4. All measured variants passed result-equivalence checks. Throughput is the median of trial throughputs; latency columns are medians of trial percentiles. Wide trial ranges make rankings provisional.\n\n';
for(const workload of workloads) {
  const source=sourceFor.get(workload);
  const {env,results}=source;
  const link=(relative(root,source.path)||'.').replaceAll('\\','/');
  md+=`## ${titles[workload]}\n\n${descriptions[workload]}\n\nSource: [${source.path.split('/').at(-1)}](${link}/environment.json), ${env.date}. ${env.rounds} rounds × ${env.iterations} operations, with ${env.warmup} warmups per trial. Axel base commit: \`${env.axelCommit}\`.\n\n`;
  const join=workload==='nested10x5'&&env.adapters.includes('axel-aql-join');
  if(join) md+=`This section uses the local TypeScript runtime strategy implementation ([patch](${link}/runtime.patch)). Aggregate is the default correlated-subquery strategy; join uses lateral JSON aggregation. Both execute one SQL statement.\n\n`;
  else md+='This section uses the original unmodified Axel implementation. Relation-strategy variants are not listed for this workload.\n\n';
  const extra=additions.filter(entry=>entry.workloads.includes(workload));
  for(const entry of extra) {
    const added=entry.source.env;
    md+=`Addition: [${entry.path.split('/').at(-1)}](${entry.path}/environment.json), ${added.date}. ${added.rounds} rounds × ${added.iterations} operations, with ${added.warmup} warmups per trial. ${entry.note} Existing measurements and dates are unchanged; this variant was measured separately.\n\n`;
  }
  for(const concurrency of env.concurrencies) {
    const entries=[...env.adapters.map(adapter=>({adapter,source})),...extra.flatMap(entry=>entry.adapters.map(adapter=>({adapter,source:entry.source})))];
    if(new Set(entries.map(entry=>entry.adapter)).size!==entries.length) throw new Error(`Duplicate variant for ${workload}`);
    const ranked=entries.map(({adapter,source:entrySource})=>{
      const {env,results}=entrySource;
      const link=(relative(root,entrySource.path)||'.').replaceAll('\\','/');
      const rows=results.filter(r=>r.adapter===adapter&&r.workload===workload&&r.concurrency===concurrency);
      if(rows.length!==env.rounds) throw new Error(`Incomplete results: ${adapter}/${workload}/${concurrency}`);
      const item={adapter,workload,concurrency,sourceRun:link,opsPerSec:median(rows.map(r=>r.opsPerSec)),minOpsPerSec:Math.min(...rows.map(r=>r.opsPerSec)),maxOpsPerSec:Math.max(...rows.map(r=>r.opsPerSec)),p50Ms:median(rows.map(r=>r.p50Ms)),p95Ms:median(rows.map(r=>r.p95Ms)),p99Ms:median(rows.map(r=>r.p99Ms))};
      summary.push(item);
      return item;
    }).sort((a,b)=>b.opsPerSec-a.opsPerSec);
    md+=`### Concurrency ${concurrency}\n\n| Implementation | ops/sec ↓ | Trial min–max ops/sec | p50 ms | p95 ms | p99 ms |\n|---|---:|---:|---:|---:|---:|\n`;
    for(const r of ranked) {
      const name=names[r.adapter]??r.adapter;
      const label=workload==='nested10x5'&&['axel-aql','axel-builder'].includes(r.adapter)?`${name} (aggregate)`:name;
      md+=`| ${label} | ${r.opsPerSec.toFixed(0)} | ${r.minOpsPerSec.toFixed(0)}–${r.maxOpsPerSec.toFixed(0)} | ${r.p50Ms.toFixed(3)} | ${r.p95Ms.toFixed(3)} | ${r.p99Ms.toFixed(3)} |\n`;
    }
    md+='\n';
  }
  if(join) {
    md+='### Axel aggregate vs join\n\nPositive change means higher median throughput with join in this run.\n\n| Mode | Concurrency | Aggregate ops/sec | Join ops/sec | Join change |\n|---|---:|---:|---:|---:|\n';
    for(const mode of ['axel-aql','axel-builder']) for(const concurrency of env.concurrencies) {
      const aggregate=summary.find(r=>r.adapter===mode&&r.concurrency===concurrency&&r.workload===workload);
      const joined=summary.find(r=>r.adapter===mode+'-join'&&r.concurrency===concurrency&&r.workload===workload);
      const change=(joined.opsPerSec/aggregate.opsPerSec-1)*100;
      md+=`| ${mode==='axel-aql'?'AQL runtime':'Query builder'} | ${concurrency} | ${aggregate.opsPerSec.toFixed(0)} | ${joined.opsPerSec.toFixed(0)} | ${change>=0?'+':''}${change.toFixed(1)}% |\n`;
    }
    md+='\n';
  }
}
md+='## Methodology and limits\n\nThese are local, warm, closed-loop database operations, including query construction, driver/pool work, PostgreSQL execution, and result decoding. Initialization, code generation, connection startup, validation, and application serialization are excluded. Pool maxima match concurrency. AQL is parsed and compiled on every call; builders are recreated for every operation. Prisma 6 uses its Rust library engine; Prisma 7 uses PrismaPg. Nested reads use join loading in Prisma and TypeORM, and Drizzle relational queries. TypeORM reads hydrate entities; inserts use its query builder with RETURNING.\n\nThe runs took place on a shared workstation with a local PostgreSQL container. Their wide trial ranges do not establish a statistically reliable or universal ranking. See the benchmark README for reproduction and the documented Axel alias-collision limitation.\n';
// Keep the run-local summary aligned with its raw trials; the combined report
// has a separate summary with explicit per-workload provenance.
writeFileSync(`${root}/summary.json`,JSON.stringify(summary.filter(r=>r.sourceRun==='.'),null,2));
writeFileSync(`${root}/report-summary.json`,JSON.stringify(summary,null,2));
writeFileSync(`${root}/REPORT.md`,md);
console.log(md);
