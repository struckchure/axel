# Axel Roadmap

Axel is a multi-language database tool designed to provide Prisma-like developer experience with advanced modeling capabilities and SQL migration generation.

## Phase 1: Core Foundation (In Progress)

### Schema Definition & Type Safety
- [x] Design and implement schema definition language (SDL)
- [ ] Create Go type generation from schema definitions
- [ ] Implement type-safe query client generation
- [x] Support for basic data types (String, Int, Boolean, DateTime, Float, JSON)
- [x] Relationship modeling (One-to-One, One-to-Many, Many-to-Many)

### SQL Migration Generation
- [x] Build migration engine core
- [x] Implement CREATE TABLE generation
- [x] Implement ALTER TABLE operations
- [ ] Generate CREATE INDEX statements
- [x] Support for constraints (PRIMARY KEY, FOREIGN KEY, UNIQUE, NOT NULL)
- [x] Migration history tracking and versioning
- [x] Rollback capability

### Type-Safe Query Client
- [ ] Basic CRUD operations (Create, Read, Update, Delete)
- [ ] Query builder with fluent API
- [ ] Type-safe select projections
- [ ] Where clause filtering with runtime validation
- [ ] Support for joins with proper type inference
- [ ] Pagination support (skip, take)
- [ ] Sorting/ordering

## Phase 2: Advanced Modeling Features

### Enhanced Schema Capabilities
- [ ] Field-level validation rules
- [ ] Custom field modifiers and decorators
- [ ] Default values and auto-generated fields
- [ ] Computed fields
- [ ] Field encryption support
- [ ] Soft deletes
- [ ] Timestamps (createdAt, updatedAt)

### Advanced Query Features
- [ ] Aggregation functions (count, sum, avg, min, max)
- [ ] Grouping and having clauses
- [ ] Subquery support
- [ ] Complex nested selects
- [ ] Transaction support
- [ ] Batch operations
- [ ] Raw query execution with type safety

### Database Features
- [ ] PostgreSQL and SQLite3 backend support
- [ ] Connection pooling
- [ ] Seed file generation and execution
- [ ] Database introspection and push
- [ ] Schema introspection from existing databases

## Phase 3: Developer Experience

### CLI Tools
- [ ] `axel init` - Initialize new project
- [ ] `axel generate` - Generate migrations and types
- [ ] `axel migrate` - Run pending migrations
- [ ] `axel studio` - Visual database explorer
- [ ] `axel seed` - Run seed files
- [ ] `axel validate` - Validate schema

### Code Generation
- [ ] Multi-language support (TypeScript, JavaScript, Python)
- [ ] Generate API routes from schema
- [ ] Generate GraphQL types and resolvers
- [ ] Generate REST endpoints
- [ ] Generate test fixtures

### Documentation & IDE Support
- [ ] IntelliSense support for VSCode
- [ ] Schema validation and linting
- [ ] Helpful error messages
- [ ] Auto-complete for query building
- [ ] Schema documentation generation

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
