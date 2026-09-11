import { writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
const model = `
model Account {
  id Int @id
  email String
  age Int
  active Boolean
  posts Post[]
  @@map("account")
}
model Post {
  id Int @id
  title String
  author Int
  user Account @relation(fields: [author], references: [id])
  @@map("post")
}
`;
for (const major of [6, 7]) {
  writeFileSync(`prisma${major}/schema.prisma`, `generator client {
  provider = "${major === 6 ? 'prisma-client-js' : 'prisma-client'}"
  output = "./generated"
  ${major === 6 ? 'engineType = "library"' : ''}
  previewFeatures = ["relationJoins"]
}
datasource db {
  provider = "postgresql"
  ${major === 6 ? 'url = env("BENCH_DATABASE_URL")' : ''}
}
${model}`);
  execFileSync(process.execPath, [`prisma${major}/node_modules/prisma/build/index.js`, 'generate', '--schema', `prisma${major}/schema.prisma`], {stdio:'inherit'});
}
execFileSync(process.env.AXEL_BIN ?? '/tmp/axel-benchmark', ['codegen','--dir','.', '--schema-path','schema.asl','-g','ts','-o','generated/axel','--option','client=pg'], {stdio:'inherit'});
