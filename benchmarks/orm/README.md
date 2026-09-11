# Axel vs Prisma 6/7, TypeORM, and Drizzle

An executable PostgreSQL benchmark of Axel's **generated TypeScript runtime AQL executor** (`Runner.run`) and **fluent query builder** (`Runner.select`/`insert`). Both construct SQL during every measured operation. This does not benchmark AOT-generated query functions, the Go `core/runner`, or CLI process startup.

Measured results: [REPORT.md](results/node24-pg18/REPORT.md). Raw trial distributions, environment metadata, and summaries are in the same directory. [Verification results](results/verification-pg18) contain captured SQL; their short timings are smoke checks, not performance results.

The original full-workload comparison contains six entries: both Axel modes, Prisma 6, Prisma 7, TypeORM, and Drizzle. `pg` is only a driver and an untimed correctness-check helper. The saved measurements were filtered to remove its former benchmark entry; retained timings were not rerun or changed. The original collection rotated seven entries; the newer nested comparison adds the two Axel join entries.

## Combined report layout

The report in `results/nested-strategies/REPORT.md` includes point reads, 100-row
reads, nested relations, and inserts, each with concurrency-1 and concurrency-10
tables sorted by throughput descending. Its `report-sources.json` includes the
original run's non-nested workloads; nested results use the newer strategy run.
Measurements from different runs are not pooled. `summary.json` remains local to
the raw run; `report-summary.json` covers every displayed workload with source paths.
Regenerate the combined report with `pnpm report results/nested-strategies`.

## Aggregate vs join nested comparison

[Fresh nested strategy results](results/nested-strategies/REPORT.md) compare four
Axel entries (`axel-aql`, `axel-aql-join`, `axel-builder`, `axel-builder-join`) plus
Prisma 6/7, TypeORM, and Drizzle. Unsuffixed Axel entries use default `query`
correlated aggregates. Join entries use `LEFT JOIN LATERAL` with JSON aggregation.
Both strategies are one SQL statement and return identical nested data.

The original TypeScript runtime did not expose join loading. This comparison uses
the local generator change adding `new Runner(pool, {relLoadStrategy: "join"})`.
It is a benchmark of that working-tree implementation, not the unchanged release.
The runtime patch is saved with the results; generated-client hashes are recorded.
The earlier full-workload report remains a historical snapshot.

```sh
BENCH_CAPTURE=1 BENCH_WORKLOADS=nested10x5 BENCH_ROUNDS=1 BENCH_ITERATIONS=20 BENCH_WARMUP=5 BENCH_OUTPUT=results/verification-strategies pnpm bench
BENCH_WORKLOADS=nested10x5 BENCH_ROUNDS=8 BENCH_ITERATIONS=2000 BENCH_WARMUP=300 BENCH_OUTPUT=results/nested-strategies pnpm bench
pnpm report results/nested-strategies
```

This fresh run measures 256,000 nested operations: eight entries × eight rounds ×
two concurrency levels × 2,000 operations. All eight positions are rotated evenly.
SQL is captured for validation only; timed query logging remains disabled.

## Pinned environment

| Component | Version / configuration |
|---|---|
| Node.js | 24.21.0 (24 LTS), local executable installed with the workspace |
| pnpm | 10.23.0 |
| PostgreSQL | 18.6, pinned Alpine container digest in compose.yaml |
| Axel | repository commit recorded in each environment.json |
| Prisma 6 | 6.19.3, Rust **library** engine, relationJoins preview enabled |
| Prisma 7 | 7.10.0, PrismaPg adapter, relationJoins preview enabled |
| TypeORM | 1.1.1, EntitySchema/repository reads; insert builder with RETURNING |
| Drizzle | 0.45.2, node-postgres driver, relational query API for nested reads |
| node-postgres | 8.16.3 (also resolved by PrismaPg and TypeORM in pnpm-lock.yaml) |

## Reproduce

Run these commands from `benchmarks/orm` with Docker and Go available:

```sh
pnpm install --frozen-lockfile
go build -o /tmp/axel-benchmark ../../cmd
docker compose up -d --wait
pnpm generate
pnpm seed
BENCH_CAPTURE=1 BENCH_ROUNDS=1 BENCH_ITERATIONS=20 BENCH_WARMUP=5 BENCH_OUTPUT=results/verification-pg18 pnpm bench
BENCH_OUTPUT=results/node24-pg18 pnpm bench
pnpm report results/node24-pg18
docker compose down -v
```

The pnpm scripts select the local Node 24.21.0 executable even if the shell's Node differs. `environment.json` records the actual benchmark runtime. Prisma versions are separate workspace packages to avoid mixing client runtimes. Generated clients are ignored and must be regenerated after checkout. Dependency versions are locked by pnpm. All documented installation and run commands use pnpm.

`AXEL_BIN` overrides `/tmp/axel-benchmark` for generation. `BENCH_DATABASE_URL` overrides the dedicated local connection. Seed **drops and recreates** `account` and `post`; the harness requires the database name `axel_benchmark` and must only target a disposable benchmark database. Credentials in this folder are local disposable defaults.

## Workloads and measurement

Seed: 10,000 accounts, five posts per account (50,000 total), primary-key indexes, and an index on `post.author`. Deterministic IDs select different parts of the data. There are no null fields or childless accounts in the measured fixture.

| Workload | Returned result |
|---|---|
| point | One account by primary key: id, email, age, active |
| range100 | 100 accounts within a primary-key range, same four fields |
| nested10x5 | 10 accounts with id/email and five nested posts each, post id/title |
| insert | One inserted account, all four fields returned |

All implementations run against the same schema. Prisma and TypeORM explicitly use join relation loading; Drizzle uses its native relational API. The verification run captures one SQL statement per operation for every adapter. There is no artificial N+1 workload. SQL shapes differ (JSON vs JSONB aggregation vs flat join hydration), which is part of the end-to-end client comparison.

Before each workload in every trial, results are compared to raw pg, sorting only for validation because ordering is not requested. Reads check three different ranges/IDs and every nested field; inserts check returned and persisted fields. Timing excludes these checks. Timed operations consume decoded results and check top-level cardinality. Inserts use unique explicit IDs; inserted rows are deleted outside measurement, including between validation, warmup, and timing.

The original report retains seven rounds × six entries × two concurrency levels × four workloads × 1,000 measured operations = **336,000 measured operations**, plus 100 warmups per workload/trial and correctness checks. Each adapter/concurrency/round runs in a fresh process. Adapters rotate by one position each round. Concurrency order alternates. Pool maxima equal concurrency (1 or 10). Connections are warmed before timing. Queries are driven by a closed loop with no think time; this measures achieved throughput and service latency, not open-loop queuing or saturation capacity.

Measured time includes query construction, pooling, driver protocol, database execution, and decoding/hydration. Initialization, imports, client generation, connection startup, and validation are excluded. Prisma 6's own internal preparation remains enabled; no manually prepared pg/Drizzle queries or cached Axel compilation are introduced. SQL event logging is only enabled with `BENCH_CAPTURE=1`; do not use that setting for final timings.

The report presents median trial throughput, its observed min–max, and the median of trial p50/p95/p99 values. Every sorted operation latency is retained in JSON. This is one shared Apple M4 workstation, with a local PostgreSQL container and other applications running. Small differences can be noise; no isolated-server, cold-start, memory, wide-row, transaction, update/delete, remote-network, or production-scale claims are implied.

New default full runs use eight entries and eight rounds. `BENCH_WORKLOADS` selects a comma-separated subset of workloads. Override `BENCH_ROUNDS`, `BENCH_ITERATIONS`, `BENCH_WARMUP`, `BENCH_CONCURRENCIES` (comma-separated), `BENCH_ADAPTERS` (comma-separated), or `BENCH_OUTPUT` as needed. Use a fresh output directory when changing run settings; the report rejects incomplete trial counts.

## Correctness finding discovered during setup

The original fixture used `BenchUser` and `BenchPost`. Runtime AQL generated alias `b` for both, so this correlated query returned Post 1 for **every** user instead of each user's five posts:

```aql
select BenchUser {
  id, email,
  posts := (select BenchPost { id, title } filter .author = BenchUser.id)
} filter .id >= $lo and .id < $hi;
```

The runtime's `_compileSelectAST` derives aliases from the first letter of the type. The inner predicate became `b."author" = b."id"`; both names refer to the inner table, losing the outer correlation. The initial correctness gate failed and those timings were rejected. The measured fixture uses `Account`/`Post` and passes full result checks. The original report used unchanged Axel source. The newer strategy comparison adds the runtime strategy option but does not fix this alias collision. Same-initial and self-referential nested types need a separate correctness fix before relying on those query forms.

The generated TypeScript AQL parser also supports less syntax than the Go compiler: it handles select/insert/delete but has no update parser, and its select parser does not implement ordering/limit clauses. This benchmark uses its supported common subset. It does not establish full AQL feature parity.

## API references

- [Prisma 7](https://www.prisma.io/docs/orm/v7)
- [Prisma 7 adapter configuration](https://www.prisma.io/docs/guides/upgrade-prisma-orm/v7)
- [TypeORM data sources](https://typeorm.io/docs/data-source/data-source-options/)
- [Drizzle PostgreSQL driver](https://orm.drizzle.team/docs/get-started-postgresql)

## Additional Prisma 7 relation strategy

`prisma7-query` selects `relationLoadStrategy: "query"`. The existing `prisma7`
entry explicitly uses `"join"`; its measurements and report labels are preserved.
Axel already has aggregate and join entries for both its AQL runtime and builder.
Prisma's `query` mode differs from Axel's default `query`/aggregate mode: Prisma
executes separate parent/child queries and assembles relations in the client,
while Axel emits one SQL statement with correlated JSON aggregates.

The additional Prisma 7 measurements live in `results/prisma7-query`, with their
actual collection date. `report-sources.json` adds only their nested rows to the
existing report. Existing dates, rows, formatting, and measurements are retained.
They were collected separately, so timing differences can include environmental
variation between runs. SQL capture in `results/verification-prisma7-query`
confirms two statements and full result equivalence for this fixture.

```sh
BENCH_ADAPTERS=prisma7-query BENCH_WORKLOADS=nested10x5 BENCH_CAPTURE=1 BENCH_ROUNDS=1 BENCH_ITERATIONS=20 BENCH_WARMUP=5 BENCH_OUTPUT=results/verification-prisma7-query pnpm bench
BENCH_ADAPTERS=prisma7-query BENCH_WORKLOADS=nested10x5 BENCH_ROUNDS=8 BENCH_ITERATIONS=2000 BENCH_WARMUP=300 BENCH_OUTPUT=results/prisma7-query pnpm bench
pnpm report results/nested-strategies
```

Prisma documents both strategies in its [v7 API reference](https://www.prisma.io/docs/orm/v7/reference/prisma-client-reference).
They are also available in [v6](https://www.prisma.io/docs/orm/v6/prisma-client/queries/relation-queries);
only the requested Prisma 7 variant was added here.
