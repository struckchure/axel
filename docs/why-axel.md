# Why Axel?

PostgreSQL is one of the most powerful, battle-tested, and feature-rich database platforms in the world. It provides native support for complex relational schemas, JSON aggregation, custom extensions (`pgvector`, `pg_trgm`, `postgis`), triggers, stored procedures, row-level security (RLS), and sophisticated query optimization.

Yet modern application development frequently struggles with the layers placed between our code and PostgreSQL.

---

## The ORM Dilemma

Every generation of Object-Relational Mappers (ORMs) and query builders has tried to solve the database interface problem, but they introduce compounding tradeoffs:

### 1. Framework-Coupled ORMs (Django, Rails Active Record)
* **The Problem:** Deeply embedded inside specific language and web framework runtimes.
* **The Reality:** They abstract the relational model so heavily that writing queries no longer feels like SQL. As queries grow in complexity, debugging the magic, fighting lazy loading, and avoiding accidental N+1 queries requires endless workarounds or dropping into raw SQL strings.

### 2. Convoluted Relationship Models (TypeORM, Sequelize)
* **The Problem:** Heavy reliance on entity decorators, bidirectional mapping, and ownership annotations.
* **The Reality:** Modeling simple one-to-many or many-to-many relationships turns into mental gymnastics (`@JoinTable`, `@ManyToOne`, `@JoinColumn`, cascading options). A single misconfigured decorator leads to silent data bugs or broken cascade updates at runtime.

### 3. TypeScript Query Builders (Drizzle, Kysely)
* **The Problem:** Excellent for type inference and writing SQL expressions in TypeScript, but relational modeling feels estranged.
* **The Reality:** Defining relationships requires separate `relations()` helper objects disconnected from the column schema definitions. Querying related data across many-to-many links feels ad-hoc and bolted-on rather than a natural part of the query syntax.

### 4. Black-Box Runtime Clients (Prisma)
* **The Problem:** An intermediate Rust binary engine running as a sidecar process.
* **The Reality:** Significant runtime overhead and memory consumption, high latency on serverless platforms, and sub-optimal query generation. Fundamental SQL operations like insert/update conflict handling (`ON CONFLICT DO UPDATE` / custom upserts) are either unsupported, restricted to unique indices, or require messy raw SQL workarounds.

### 5. Neglected PostgreSQL Capabilities
* **The Problem:** Most ORMs treat databases as generic, lowest-common-denominator storage engines.
* **The Reality:** While tools like Drizzle have added basic `pgPolicy` helpers, declarative support across conventional ORMs for PostgreSQL extensions (`pgvector`, `pg_trgm`, `postgis`, `citext`), database triggers (`BEFORE/AFTER INSERT/UPDATE`), custom procedures, partial indexes, and conditional aggregates (`FILTER (WHERE ...)`) is largely missing, forcing teams to maintain out-of-band manual SQL migration scripts.

### 6. Enterprise Demands
* Traditional ORMs struggle to deliver the clean, deterministic, and predictable SQL required by high-throughput systems, database administrators, and enterprise compliance standards.

---

## The Axel Solution

Axel re-architects database tooling with a fundamental insight: **data modeling and query compilation belong at build time, not inside a heavy runtime library.**

```
┌────────────────────────┐      ┌────────────────────────┐
│  schema.asl  (Schema)  │      │   query.aql (Query)    │
└───────────┬────────────┘      └───────────┬────────────┘
            │                               │
            ▼                               ▼
     axel generate                    axel compile
            │                               │
            ▼                               ▼
    migration.sql                   parameterized SQL
   (applied to DB)             (executed by your native driver)
```

---

## Key Advantages

### 1. Compiler, Not a Runtime ORM
Axel never wraps a database driver, manages a connection pool, or executes queries on your behalf.
* **ASL** compiles into clean, deterministic SQL migrations with checksum history tracking.
* **AQL** compiles into parameterized, optimal PostgreSQL query strings (`$1`, `$2`, ...).
* You execute the generated SQL using your language's standard, high-performance driver (`pgx`, `node-postgres`, `sqlx`, `bun`, etc.).

### 2. First-Class Relational Modeling
Relationships are first-class declarations in ASL, not awkward metadata:

```asl
type User {
  required id: uuid;
  required email: str;
}

type Post {
  required id: uuid;
  required title: str;
  required link author: User;    # Foreign key column
  multi link likes: User;        # Automatic junction table
}
```

Junction tables, foreign key constraints, indexes, and cascades are generated automatically.

### 3. Zero N+1 Queries via JSON Aggregation
In traditional ORMs, selecting nested relational graphs either executes $N+1$ separate queries or produces giant Cartesian products across multiple `JOIN`s.

Axel compiles nested AQL shapes directly into PostgreSQL lateral subqueries using `json_agg` and `row_to_json`:

```aql
select Post {
  id,
  title,
  author: { id, email },
  likes: { id, email }
}
filter .author.id = $author_id;
```

PostgreSQL executes this in a **single scan**, returning fully shaped JSON objects directly to your application with zero runtime assembling overhead.

### 4. Native PostgreSQL as a First-Class Citizen
Axel is built specifically for PostgreSQL. It supports advanced PostgreSQL features natively in your schema and query files:

* **Extensions:** `extension pgcrypto;`, `extension pg_trgm;`, `extension vector;`
* **Row-Level Security (RLS):** `policy user_isolation on Document for all using (.owner = global.current_user);`
* **Triggers & Functions:** Declarative triggers that update columns before/after mutations.
* **Upsert Conflicts:** `insert User { ... } unless conflict on .email else (update User set { ... });`
* **CTE `with` Bindings:** Reusable subquery bindings compiled to `WITH _with_<name> AS (...)`.
* **Aggregates & Grouping:** Native `group by`, `having`, and per-field conditional filters `sum(.amount)<int64> filter .status = 'Completed'`.

### 5. Universal Language Support & Generators
Because Axel compiles directly to pure SQL, it can be used with **any programming language or runtime**. Axel includes first-party code generators for Go (`pgx`) and TypeScript, and makes it easy to add new languages by writing a generator in Go (or as an external binary plugin):

* **Type Safety Everywhere:** Fully typed parameter structs/interfaces and exact row response types generated straight from your AQL query shapes.
* **Write Your Own Generator:** You can implement Axel's generator interface in Go to target any language (e.g. Rust, Python, C#, Java, PHP, Elixir).
* **Missing Your Language?** If a generator for your language is not yet built-in, please [file an issue on GitHub](https://github.com/struckchure/axel/issues) — we'd love to add official support!

---

## Comparison Matrix

| Feature | Django ORM | TypeORM | Prisma | Drizzle | Axel |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Architecture** | Runtime ORM | Runtime ORM | Rust Engine Sidecar | TypeScript Builder | **AOT Compiler** |
| **Runtime Driver Overhead** | High | High | Very High | Zero | **Zero** |
| **Relational Shapes** | Lazy / Prefetch | Manual Joins | Nested Roundtrips | `relations()` helper | **Native `json_agg` in 1 query** |
| **Insert / Update Conflicts** | Verbose | Awkward | Partial / Raw SQL | Supported | **Native `unless conflict`** |
| **PostgreSQL Extensions** | Manual SQL | Manual SQL | Manual SQL | Manual SQL | **First-class `extension`** |
| **Row-Level Security (RLS)** | No | No | No | Supported (`pgPolicy`) | **First-class `policy`** |
| **Database Triggers** | No | Decorators / Hooks | No | No | **First-class `trigger`** |
| **Multi-Language Support** | Python Only | JS/TS Only | JS/TS / Go / Rust | TS/JS Only | **Any Language (Go, TS, +)** |
| **Interactive Studio** | Django Admin | None | Prisma Studio | Drizzle Studio | **Axel Studio (`axel studio`)** |
