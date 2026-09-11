# Axel ORM benchmark

Results are grouped by workload, with separate concurrency-1 and concurrency-10 tables. Every table is sorted by median throughput, highest first. **ops/sec means completed operations per second.**

This report combines saved runs: point reads, 100-row reads, and inserts come from the original full-workload run; nested relations come from the newer aggregate-versus-join run. No measurements were rerun or pooled across runs. Each section identifies its source and implementation.

All runs use pnpm, Node 24.21.0, PostgreSQL 18.6, and the same 10,000-account / 50,000-post fixture on an Apple M4. All measured variants passed result-equivalence checks. Throughput is the median of trial throughputs; latency columns are medians of trial percentiles. Wide trial ranges make rankings provisional.

## Point reads

Read one account by primary key, returning id, email, age, and active.

Source: [node24-pg18](../node24-pg18/environment.json), 2026-09-11T20:18:25.529Z. 7 rounds × 1000 operations, with 100 warmups per trial. Axel base commit: `0fa2ca54925e22e41637a4bad63b0d9c7d382616`.

This section uses the original unmodified Axel implementation. Relation-strategy variants are not listed for this workload.

### Concurrency 1

| Implementation | ops/sec ↓ | Trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Prisma 6 | 3439 | 588–3976 | 0.230 | 0.828 | 1.426 |
| Axel builder | 2423 | 889–6288 | 0.244 | 1.241 | 2.149 |
| Axel AQL | 1954 | 1102–6100 | 0.329 | 1.342 | 2.558 |
| Prisma 7 | 1464 | 647–4158 | 0.394 | 1.880 | 3.292 |
| Drizzle | 1033 | 837–1713 | 0.602 | 2.657 | 4.937 |
| TypeORM | 946 | 651–1662 | 0.667 | 2.872 | 4.517 |

### Concurrency 10

| Implementation | ops/sec ↓ | Trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Axel builder | 6520 | 2671–13638 | 0.986 | 4.966 | 6.919 |
| Prisma 7 | 6263 | 1978–19481 | 1.445 | 3.256 | 4.812 |
| Axel AQL | 6114 | 1281–8831 | 1.336 | 3.890 | 5.594 |
| Prisma 6 | 5189 | 2091–17209 | 1.895 | 3.216 | 3.946 |
| TypeORM | 4569 | 1925–13251 | 1.600 | 5.331 | 7.978 |
| Drizzle | 3536 | 2054–5500 | 2.118 | 7.303 | 12.519 |

## 100-row reads

Read 100 accounts in a primary-key range, returning the same four fields.

Source: [node24-pg18](../node24-pg18/environment.json), 2026-09-11T20:18:25.529Z. 7 rounds × 1000 operations, with 100 warmups per trial. Axel base commit: `0fa2ca54925e22e41637a4bad63b0d9c7d382616`.

This section uses the original unmodified Axel implementation. Relation-strategy variants are not listed for this workload.

### Concurrency 1

| Implementation | ops/sec ↓ | Trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Axel builder | 1830 | 710–3204 | 0.371 | 1.671 | 2.541 |
| Drizzle | 1372 | 704–2120 | 0.472 | 1.900 | 3.445 |
| Axel AQL | 1308 | 423–3124 | 0.547 | 2.115 | 3.070 |
| TypeORM | 1283 | 669–1888 | 0.521 | 1.957 | 3.517 |
| Prisma 7 | 1100 | 676–2967 | 0.562 | 2.656 | 4.319 |
| Prisma 6 | 1009 | 383–1860 | 0.555 | 2.726 | 4.770 |

### Concurrency 10

| Implementation | ops/sec ↓ | Trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Axel builder | 4094 | 1490–8677 | 2.035 | 5.489 | 7.274 |
| Axel AQL | 3646 | 1184–6566 | 2.006 | 5.397 | 14.819 |
| Prisma 6 | 2412 | 1054–7273 | 2.531 | 9.264 | 18.837 |
| Drizzle | 1987 | 1352–4389 | 3.854 | 10.112 | 18.995 |
| TypeORM | 1675 | 907–6292 | 5.148 | 12.639 | 17.495 |
| Prisma 7 | 1226 | 949–2317 | 5.921 | 17.910 | 32.615 |

## Nested relations

Read 10 accounts with five posts each, returning account id/email and post id/title.

Source: [nested-strategies](./environment.json), 2026-09-11T20:41:35.440Z. 8 rounds × 2000 operations, with 300 warmups per trial. Axel base commit: `0fa2ca54925e22e41637a4bad63b0d9c7d382616`.

This section uses the local TypeScript runtime strategy implementation ([patch](./runtime.patch)). Aggregate is the default correlated-subquery strategy; join uses lateral JSON aggregation. Both execute one SQL statement.

Addition: [prisma7-query](../prisma7-query/environment.json), 2026-09-11T21:12:01.772Z. 8 rounds × 2000 operations, with 300 warmups per trial. The existing Prisma 7 rows use join loading. Prisma 7 (query) uses two SQL queries and combines relations in the client. Axel aggregate and join variants above remain unchanged. Existing measurements and dates are unchanged; this variant was measured separately.

### Concurrency 1

| Implementation | ops/sec ↓ | Trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Axel builder (aggregate) | 1744 | 436–4097 | 0.432 | 1.721 | 3.179 |
| Prisma 7 (query) | 1491 | 503–2645 | 0.432 | 1.673 | 3.392 |
| Axel builder (join) | 1476 | 490–4263 | 0.415 | 2.440 | 4.765 |
| Axel AQL (aggregate) | 1474 | 267–4265 | 0.367 | 2.091 | 5.262 |
| Axel AQL (join) | 1389 | 575–4025 | 0.392 | 2.089 | 4.726 |
| TypeORM | 1113 | 429–2797 | 0.516 | 2.572 | 4.441 |
| Prisma 7 | 1045 | 349–2678 | 0.580 | 2.666 | 5.536 |
| Drizzle | 956 | 523–2433 | 0.569 | 3.268 | 5.502 |
| Prisma 6 | 694 | 363–2238 | 0.980 | 3.848 | 7.029 |

### Concurrency 10

| Implementation | ops/sec ↓ | Trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Axel builder (join) | 5345 | 1789–12533 | 1.935 | 6.388 | 9.762 |
| Prisma 7 (query) | 5239 | 1599–5706 | 1.578 | 3.573 | 6.899 |
| Axel builder (aggregate) | 4087 | 1778–9746 | 1.871 | 5.993 | 10.547 |
| Axel AQL (aggregate) | 3874 | 1491–9302 | 1.853 | 5.898 | 11.628 |
| Axel AQL (join) | 3530 | 1445–10875 | 2.273 | 6.435 | 9.871 |
| Prisma 6 | 2182 | 1354–5742 | 3.271 | 8.377 | 17.012 |
| TypeORM | 2043 | 1246–7035 | 4.008 | 11.896 | 17.189 |
| Drizzle | 1928 | 1230–5037 | 4.043 | 11.801 | 20.961 |
| Prisma 7 | 1906 | 1243–4793 | 3.362 | 13.663 | 23.021 |

### Axel aggregate vs join

Positive change means higher median throughput with join in this run.

| Mode | Concurrency | Aggregate ops/sec | Join ops/sec | Join change |
|---|---:|---:|---:|---:|
| AQL runtime | 1 | 1474 | 1389 | -5.8% |
| AQL runtime | 10 | 3874 | 3530 | -8.9% |
| Query builder | 1 | 1744 | 1476 | -15.3% |
| Query builder | 10 | 4087 | 5345 | +30.8% |

## Inserts

Insert one account and return all four fields.

Source: [node24-pg18](../node24-pg18/environment.json), 2026-09-11T20:18:25.529Z. 7 rounds × 1000 operations, with 100 warmups per trial. Axel base commit: `0fa2ca54925e22e41637a4bad63b0d9c7d382616`.

This section uses the original unmodified Axel implementation. Relation-strategy variants are not listed for this workload.

### Concurrency 1

| Implementation | ops/sec ↓ | Trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Axel builder | 515 | 207–818 | 1.534 | 4.131 | 7.631 |
| Axel AQL | 405 | 219–845 | 2.083 | 4.237 | 7.929 |
| Prisma 6 | 359 | 252–714 | 2.465 | 4.711 | 7.781 |
| Drizzle | 336 | 169–741 | 2.358 | 5.222 | 8.089 |
| TypeORM | 336 | 186–777 | 2.427 | 5.646 | 12.585 |
| Prisma 7 | 329 | 259–397 | 2.207 | 5.893 | 10.322 |

### Concurrency 10

| Implementation | ops/sec ↓ | Trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Axel builder | 2134 | 1723–3175 | 3.699 | 8.634 | 14.800 |
| Prisma 7 | 2042 | 858–4147 | 4.365 | 8.119 | 28.345 |
| Axel AQL | 1698 | 1176–4848 | 4.715 | 10.979 | 24.337 |
| Drizzle | 1615 | 1293–3495 | 5.436 | 11.199 | 16.270 |
| Prisma 6 | 1558 | 1131–3530 | 5.134 | 13.989 | 21.264 |
| TypeORM | 1408 | 1136–3216 | 6.084 | 14.448 | 25.406 |

## Methodology and limits

These are local, warm, closed-loop database operations, including query construction, driver/pool work, PostgreSQL execution, and result decoding. Initialization, code generation, connection startup, validation, and application serialization are excluded. Pool maxima match concurrency. AQL is parsed and compiled on every call; builders are recreated for every operation. Prisma 6 uses its Rust library engine; Prisma 7 uses PrismaPg. Nested reads use join loading in Prisma and TypeORM, and Drizzle relational queries. TypeORM reads hydrate entities; inserts use its query builder with RETURNING.

The runs took place on a shared workstation with a local PostgreSQL container. Their wide trial ranges do not establish a statistically reliable or universal ranking. See the benchmark README for reproduction and the documented Axel alias-collision limitation.
