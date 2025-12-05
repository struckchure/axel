# Axel

Multi-Language Database Tool aiming at [Gel](https://www.geldata.com)'s DX

Axel is a powerful command-line tool that introspects database schemas and generates type-safe code in multiple programming languages. It helps improve developer experience by automating the creation of database models.

## Features

- 🗄️ **Multi-Database Support**: PostgreSQL, MySQL, and SQLite
- 🌐 **Multi-Language Code Generation**: Go, Python, TypeScript, and JavaScript
- 🔍 **Schema Introspection**: Automatically discover database structure
- ⚡ **Developer-Friendly**: Simple YAML configuration
- 🎯 **Type-Safe**: Generates strongly-typed models for supported languages

## Installation

### From Source

```bash
go install github.com/struckchure/axel/cmd/axel@latest
```

### Build Locally

```bash
git clone https://github.com/struckchure/axel.git
cd axel
go build -o axel ./cmd/axel
```

## Quick Start

1. **Initialize a new configuration:**

```bash
axel init
```

This creates an `axel.yaml` configuration file.

2. **Edit the configuration** to match your database:

```yaml
database:
  type: postgres
  host: localhost
  port: 5432
  database: mydb
  username: user
  password: password
  sslMode: disable
generators:
  - language: go
    options:
      package: models
  - language: python
  - language: typescript
  - language: javascript
output:
  directory: ./generated
```

3. **Introspect your database schema:**

```bash
axel introspect
```

4. **Generate code from your schema:**

```bash
axel generate
```

## Supported Databases

- **PostgreSQL** (`type: postgres`)
- **MySQL** (`type: mysql`)
- **SQLite** (`type: sqlite`)

## Supported Languages

### Go

Generates Go structs with appropriate tags:

```go
type User struct {
    ID        int       `json:"id" db:"id"`
    Username  string    `json:"username" db:"username"`
    Email     string    `json:"email" db:"email"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
}
```

### Python

Generates Python dataclasses:

```python
@dataclass
class User:
    """Represents the users table"""
    id: int
    username: str
    email: str
    created_at: datetime
```

### TypeScript

Generates TypeScript interfaces:

```typescript
export interface User {
  id: number;
  username: string;
  email: string;
  createdAt: Date;
}
```

### JavaScript

Generates JavaScript classes:

```javascript
class User {
  constructor(data = {}) {
    this.id = data.id;
    this.username = data.username;
    this.email = data.email;
    this.createdAt = data.createdAt;
  }
}
```

## Commands

### `axel init`

Creates a default `axel.yaml` configuration file in the current directory.

### `axel introspect`

Connects to the database and displays the schema structure including tables and columns.

### `axel generate`

Generates code in the configured languages based on the database schema.

### `axel version`

Shows the current version of Axel.

### `axel help`

Displays help information about available commands.

## Configuration

The `axel.yaml` file supports the following options:

### Database Configuration

```yaml
database:
  type: postgres        # Database type: postgres, mysql, or sqlite
  host: localhost       # Database host
  port: 5432           # Database port
  database: mydb       # Database name
  username: user       # Database username
  password: password   # Database password
  sslMode: disable     # SSL mode (postgres only)
```

For SQLite, only the `database` field is required (path to the database file).

### Generator Configuration

```yaml
generators:
  - language: go       # Language to generate
    options:
      package: models  # Package name (Go only)
  - language: python
  - language: typescript
  - language: javascript
```

### Output Configuration

```yaml
output:
  directory: ./generated  # Output directory for generated files
```

## Examples

See the [examples](./examples) directory for complete examples with different database configurations.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

Built to improve the developer experience inspired by [Gel](https://www.geldata.com)
