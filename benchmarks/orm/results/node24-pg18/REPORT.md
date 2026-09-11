# Axel ORM benchmark

Run: 2026-09-11T20:18:25.529Z. Axel commit: `0fa2ca54925e22e41637a4bad63b0d9c7d382616`.

Node v24.21.0, Apple M4, darwin/arm64, PostgreSQL 18.6 on aarch64-unknown-linux-musl, compiled by gcc (Alpine 15.2.0) 15.2.0, 64-bit.

7 rounds, 1000 measured operations and 100 warmups per workload per trial. 10,000 accounts, 50,000 posts. All equivalence checks passed. Throughput values are median trial operations/sec; latency values are medians of each trial's percentile, not pooled percentiles.

The standalone driver entry was removed after collection. Retained ORM/runtime measurements are unchanged; the original collection rotated seven entries, while future runs rotate six.

## Throughput overview

Higher is better; operations/second. Each workload is sorted from highest to lowest median throughput. Results compare ORM/runtime and query-builder APIs. Inspect trial ranges before drawing conclusions.

Concurrency **1** — Point

| Adapter | ops/sec |
|---|---:|
| Prisma 6 | 3439 |
| Axel builder (aggregate) | 2423 |
| Axel AQL (aggregate) | 1954 |
| Prisma 7 | 1464 |
| Drizzle | 1033 |
| TypeORM | 946 |

Concurrency **1** — 100-row range

| Adapter | ops/sec |
|---|---:|
| Axel builder (aggregate) | 1830 |
| Drizzle | 1372 |
| Axel AQL (aggregate) | 1308 |
| TypeORM | 1283 |
| Prisma 7 | 1100 |
| Prisma 6 | 1009 |

Concurrency **1** — 10 accounts × 5 posts

| Adapter | ops/sec |
|---|---:|
| Drizzle | 1203 |
| Axel builder (aggregate) | 1173 |
| Axel AQL (aggregate) | 1028 |
| Prisma 7 | 894 |
| TypeORM | 769 |
| Prisma 6 | 694 |

Concurrency **1** — Insert

| Adapter | ops/sec |
|---|---:|
| Axel builder (aggregate) | 515 |
| Axel AQL (aggregate) | 405 |
| Prisma 6 | 359 |
| Drizzle | 336 |
| TypeORM | 336 |
| Prisma 7 | 329 |

Concurrency **10** — Point

| Adapter | ops/sec |
|---|---:|
| Axel builder (aggregate) | 6520 |
| Prisma 7 | 6263 |
| Axel AQL (aggregate) | 6114 |
| Prisma 6 | 5189 |
| TypeORM | 4569 |
| Drizzle | 3536 |

Concurrency **10** — 100-row range

| Adapter | ops/sec |
|---|---:|
| Axel builder (aggregate) | 4094 |
| Axel AQL (aggregate) | 3646 |
| Prisma 6 | 2412 |
| Drizzle | 1987 |
| TypeORM | 1675 |
| Prisma 7 | 1226 |

Concurrency **10** — 10 accounts × 5 posts

| Adapter | ops/sec |
|---|---:|
| Axel builder (aggregate) | 5937 |
| Axel AQL (aggregate) | 5455 |
| Prisma 7 | 2761 |
| Prisma 6 | 2516 |
| Drizzle | 2136 |
| TypeORM | 1477 |

Concurrency **10** — Insert

| Adapter | ops/sec |
|---|---:|
| Axel builder (aggregate) | 2134 |
| Prisma 7 | 2042 |
| Axel AQL (aggregate) | 1698 |
| Drizzle | 1615 |
| Prisma 6 | 1558 |
| TypeORM | 1408 |

## Concurrency 1

| Adapter | Workload | ops/sec | trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---|---:|---:|---:|---:|---:|
| Axel AQL (aggregate) | point | 1954 | 1102–6100 | 0.329 | 1.342 | 2.558 |
| Axel AQL (aggregate) | range100 | 1308 | 423–3124 | 0.547 | 2.115 | 3.070 |
| Axel AQL (aggregate) | nested10x5 | 1028 | 659–3170 | 0.611 | 2.325 | 3.702 |
| Axel AQL (aggregate) | insert | 405 | 219–845 | 2.083 | 4.237 | 7.929 |
| Axel builder (aggregate) | point | 2423 | 889–6288 | 0.244 | 1.241 | 2.149 |
| Axel builder (aggregate) | range100 | 1830 | 710–3204 | 0.371 | 1.671 | 2.541 |
| Axel builder (aggregate) | nested10x5 | 1173 | 677–1730 | 0.502 | 2.440 | 4.382 |
| Axel builder (aggregate) | insert | 515 | 207–818 | 1.534 | 4.131 | 7.631 |
| Prisma 6 | point | 3439 | 588–3976 | 0.230 | 0.828 | 1.426 |
| Prisma 6 | range100 | 1009 | 383–1860 | 0.555 | 2.726 | 4.770 |
| Prisma 6 | nested10x5 | 694 | 539–1424 | 1.227 | 2.580 | 4.575 |
| Prisma 6 | insert | 359 | 252–714 | 2.465 | 4.711 | 7.781 |
| Prisma 7 | point | 1464 | 647–4158 | 0.394 | 1.880 | 3.292 |
| Prisma 7 | range100 | 1100 | 676–2967 | 0.562 | 2.656 | 4.319 |
| Prisma 7 | nested10x5 | 894 | 467–1983 | 0.815 | 3.408 | 4.649 |
| Prisma 7 | insert | 329 | 259–397 | 2.207 | 5.893 | 10.322 |
| TypeORM | point | 946 | 651–1662 | 0.667 | 2.872 | 4.517 |
| TypeORM | range100 | 1283 | 669–1888 | 0.521 | 1.957 | 3.517 |
| TypeORM | nested10x5 | 769 | 399–2286 | 0.930 | 3.294 | 4.729 |
| TypeORM | insert | 336 | 186–777 | 2.427 | 5.646 | 12.585 |
| Drizzle | point | 1033 | 837–1713 | 0.602 | 2.657 | 4.937 |
| Drizzle | range100 | 1372 | 704–2120 | 0.472 | 1.900 | 3.445 |
| Drizzle | nested10x5 | 1203 | 425–2819 | 0.610 | 1.938 | 3.272 |
| Drizzle | insert | 336 | 169–741 | 2.358 | 5.222 | 8.089 |

## Concurrency 10

| Adapter | Workload | ops/sec | trial min–max ops/sec | p50 ms | p95 ms | p99 ms |
|---|---|---:|---:|---:|---:|---:|
| Axel AQL (aggregate) | point | 6114 | 1281–8831 | 1.336 | 3.890 | 5.594 |
| Axel AQL (aggregate) | range100 | 3646 | 1184–6566 | 2.006 | 5.397 | 14.819 |
| Axel AQL (aggregate) | nested10x5 | 5455 | 1018–11054 | 1.468 | 3.949 | 5.621 |
| Axel AQL (aggregate) | insert | 1698 | 1176–4848 | 4.715 | 10.979 | 24.337 |
| Axel builder (aggregate) | point | 6520 | 2671–13638 | 0.986 | 4.966 | 6.919 |
| Axel builder (aggregate) | range100 | 4094 | 1490–8677 | 2.035 | 5.489 | 7.274 |
| Axel builder (aggregate) | nested10x5 | 5937 | 1735–9863 | 1.342 | 3.454 | 6.416 |
| Axel builder (aggregate) | insert | 2134 | 1723–3175 | 3.699 | 8.634 | 14.800 |
| Prisma 6 | point | 5189 | 2091–17209 | 1.895 | 3.216 | 3.946 |
| Prisma 6 | range100 | 2412 | 1054–7273 | 2.531 | 9.264 | 18.837 |
| Prisma 6 | nested10x5 | 2516 | 1027–6397 | 3.061 | 9.177 | 12.578 |
| Prisma 6 | insert | 1558 | 1131–3530 | 5.134 | 13.989 | 21.264 |
| Prisma 7 | point | 6263 | 1978–19481 | 1.445 | 3.256 | 4.812 |
| Prisma 7 | range100 | 1226 | 949–2317 | 5.921 | 17.910 | 32.615 |
| Prisma 7 | nested10x5 | 2761 | 1053–3238 | 3.102 | 7.829 | 12.231 |
| Prisma 7 | insert | 2042 | 858–4147 | 4.365 | 8.119 | 28.345 |
| TypeORM | point | 4569 | 1925–13251 | 1.600 | 5.331 | 7.978 |
| TypeORM | range100 | 1675 | 907–6292 | 5.148 | 12.639 | 17.495 |
| TypeORM | nested10x5 | 1477 | 939–6034 | 5.962 | 15.415 | 26.875 |
| TypeORM | insert | 1408 | 1136–3216 | 6.084 | 14.448 | 25.406 |
| Drizzle | point | 3536 | 2054–5500 | 2.118 | 7.303 | 12.519 |
| Drizzle | range100 | 1987 | 1352–4389 | 3.854 | 10.112 | 18.995 |
| Drizzle | nested10x5 | 2136 | 1058–2430 | 4.274 | 10.837 | 14.690 |
| Drizzle | insert | 1615 | 1293–3495 | 5.436 | 11.199 | 16.270 |

## Scope

These are local, warm, closed-loop database operations, including query construction, driver/pool work, PostgreSQL execution, and result decoding. They exclude initialization, code generation, network APIs, cold starts, and application serialization. Equal concurrency/pool limits are used. Axel entries without a suffix use the default correlated aggregate strategy; `-join` entries use LEFT JOIN LATERAL aggregation. AQL is parsed/compiled on every call; the Axel builder is recreated on every call. No AOT shortcut or prepared Drizzle query is used. Prisma 6 uses its Rust library engine; Prisma 7 uses PrismaPg. Nested reads use join loading in both Prisma versions and TypeORM, and Drizzle relational queries. TypeORM reads hydrate entities; inserts use its query builder with RETURNING. SQL strategies and internal preparation may differ.

Do not extrapolate small differences into universal rankings: this is one shared workstation with PostgreSQL in a local container, not an isolated server or production network. See the benchmark README for reproduction, exact versions, and the Axel alias-collision finding.
