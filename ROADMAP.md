# Axel Roadmap

Axel is a multi-language database tool designed to provide Prisma-like developer experience with advanced modeling capabilities and SQL migration generation.

## Phase 1: Core Foundation (In Progress)

### Schema Definition & Type Safety

- [x] Design and implement schema definition language (SDL)
- [x] Create Go type generation from schema definitions
- [x] Implement type-safe query client generation (TypeScript)
- [x] Support for basic data types (String, Int, Boolean, DateTime, Float, JSON)
- [x] Relationship modeling (One-to-One, One-to-Many, Many-to-Many)

### SQL Migration Generation

- [x] Build migration engine core
- [x] Implement CREATE TABLE generation
- [x] Implement ALTER TABLE operations
- [x] Generate CREATE INDEX statements
- [x] Support for constraints (PRIMARY KEY, FOREIGN KEY, UNIQUE, NOT NULL)
- [x] Migration history tracking and versioning
- [x] Rollback capability

### Type-Safe Query Client

- [x] Basic CRUD operations (Create, Read, Update, Delete)
- [x] Query builder with fluent API
- [x] Type-safe select projections
- [x] Where clause filtering with runtime validation
- [ ] Support for joins with proper type inference
- [x] Pagination support (skip, take)
- [x] Sorting/ordering

## Phase 2: Advanced Modeling Features

### Enhanced Schema Capabilities

- [x] Field-level validation rules
- [ ] Custom field modifiers and decorators
- [x] Default values and auto-generated fields
- [x] Computed fields
- [ ] Field encryption support
- [ ] Soft deletes
- [x] Timestamps (createdAt, updatedAt) (available via rewrites/triggers)

### Advanced Query Features

- [x] Aggregation functions (count, sum, avg, min, max)
- [x] Grouping and having clauses
- [x] Subquery support
- [x] Complex nested selects
- [ ] Transaction support
- [ ] Batch operations
- [x] Raw query execution with type safety

### Database Features

- [ ] PostgreSQL and SQLite3 backend support
- [ ] Connection pooling
- [ ] Seed file generation and execution
- [ ] Database introspection and push
- [ ] Schema introspection from existing databases

## Phase 3: Developer Experience

### CLI Tools

- [ ] `axel init` - Initialize new project
- [x] `axel diff` - Generate migrations and types
- [x] `axel up` - Run pending migrations
- [x] `axel studio` - Visual database explorer
- [ ] `axel seed` - Run seed files
- [x] `axel validate` - Validate schema

### Code Generation

- [ ] Multi-language support (TypeScript, JavaScript, Python)
- [ ] Generate API routes from schema
- [ ] Generate GraphQL types and resolvers
- [ ] Generate REST endpoints
- [ ] Generate test fixtures

### Documentation & IDE Support

- [x] IntelliSense support for VSCode & Zed
- [x] Schema validation and linting
- [x] Helpful error messages
- [x] Auto-complete for query building
- [x] Schema documentation generation

## Phase 4: Advanced Features

### Performance & Optimization

- [ ] Query optimization suggestions
- [ ] Automatic index recommendations
- [ ] Query performance monitoring
- [ ] Connection pool optimization
- [ ] Caching layer support

### Extensibility

- [ ] Custom field type support
- [ ] Plugin system for extensions
- [ ] Custom validation rules
- [ ] Middleware support
- [ ] Custom generators

### Enterprise Features

- [ ] Multi-tenancy support
- [ ] Audit logging
- [ ] Row-level security (RLS)
- [ ] Data masking
- [ ] Compliance tools (GDPR, HIPAA)

## Phase 5: Ecosystem & Community

### Integrations

- [ ] Popular ORM integrations
- [ ] API framework integrations (Express, Fastify, Next.js, NestJS)
- [ ] CLI framework integrations
- [ ] Testing framework integrations

### Community & Tooling

- [ ] Package registry/marketplace
- [ ] Template repositories
- [ ] Community schema library
- [ ] Educational resources
- [ ] Migration guides from other ORMs

---

## Priority Matrix

### High Priority (Core)

- Schema definition language
- Type generation
- SQL migration generation
- Basic CRUD operations
- CLI tools

### Medium Priority (Essential)

- Advanced query features
- Multiple database backends
- IDE support
- Documentation

### Low Priority (Nice-to-have)

- Performance monitoring
- Enterprise features
- Ecosystem integrations
- Community marketplace
