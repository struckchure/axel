# Axel

**Axel** is a modern database tool primarily designed for Go, with multi-language support. It brings Prisma-like developer experience to database development, providing type-safe query clients, automatic SQL migration generation, and advanced schema modeling features.

## Features

### 🚀 Type-Safe Query Client
- Fully typed query builder with TypeScript support
- Compile-time type checking for queries
- Automatic type generation from schema
- IntelliSense and auto-complete support

### 📦 SQL Migration Generation
- Automatic migration creation from schema changes
- Support for complex database operations
- Migration history tracking
- Safe rollback capabilities
- Support for multiple databases

### 🎨 Advanced Modeling
- Flexible schema definition language
- Support for relationships (1-1, 1-N, N-N)
- Field validation and constraints
- Computed and virtual fields
- Soft deletes and timestamps

### 🌐 Multi-Language Support
- **Primary**: Go
- **Also supported**: TypeScript/JavaScript
- **Low priority for MVP**: Python and additional languages
- Consistent API across languages
- Platform-agnostic schema definition

### 📊 Multi-Database Support
- PostgreSQL
- SQLite3
- Extensible for custom databases

## Documentation

- [Getting Started](./docs/getting-started.md)
- [Schema Definition](./docs/schema-definition.md)
- [Query Client](./docs/query-client.md)
- [Migrations](./docs/migrations.md)
- [CLI Reference](./docs/cli-reference.md)

## Roadmap

See [ROADMAP.md](./ROADMAP.md) for planned features and development phases.

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

## License

See [LICENSE](./LICENSE) for details.

---

Built with ❤️ aiming at [Gel](https://www.geldata.com)'s DX
