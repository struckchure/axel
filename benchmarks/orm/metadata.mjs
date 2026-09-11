import { readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import pg from 'pg';
import {pathToFileURL} from 'node:url';
import { url } from './config.mjs';

export async function metadata() {
  const db = new pg.Client({connectionString:url});
  await db.connect();
  try {
    const settings = (await db.query("SELECT name, setting, unit FROM pg_settings WHERE name IN ('shared_buffers','work_mem','max_connections','fsync','synchronous_commit','jit','autovacuum') ORDER BY name")).rows;
    const pkg = JSON.parse(readFileSync('package.json','utf8'));
    return {
      dependencies:pkg.dependencies,
      packageManager:pkg.packageManager,
      prisma6:JSON.parse(readFileSync('prisma6/package.json','utf8')).dependencies,
      prisma7:JSON.parse(readFileSync('prisma7/package.json','utf8')).dependencies,
      lockSha256:createHash('sha256').update(readFileSync('pnpm-lock.yaml')).digest('hex'),
      generatedAxelSha256:createHash('sha256').update(readFileSync('generated/axel/runner.ts')).digest('hex'),
      postgresSettings:settings,
    };
  } finally {await db.end();}
}
if(import.meta.url === pathToFileURL(process.argv[1]).href && process.argv[2]) {
  const path=process.argv[2];
  const env=JSON.parse(readFileSync(path,'utf8'));
  writeFileSync(path,JSON.stringify({...env,...await metadata()},null,2));
}
