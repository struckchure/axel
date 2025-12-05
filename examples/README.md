# Axel Examples

This directory contains example configuration files for different database types.

## Examples

### PostgreSQL

See [postgres-example.yaml](./postgres-example.yaml) for a PostgreSQL configuration.

```bash
# Edit the configuration with your database credentials
cp postgres-example.yaml axel.yaml
# Edit axel.yaml with your details
axel introspect
axel generate
```

### MySQL

See [mysql-example.yaml](./mysql-example.yaml) for a MySQL configuration.

```bash
cp mysql-example.yaml axel.yaml
# Edit axel.yaml with your details
axel introspect
axel generate
```

### SQLite

See [sqlite-example.yaml](./sqlite-example.yaml) for a SQLite configuration.

```bash
cp sqlite-example.yaml axel.yaml
# Edit axel.yaml with your database path
axel introspect
axel generate
```

## Creating a Test Database

### SQLite Example

```bash
# Create a test SQLite database
sqlite3 test.db <<EOF
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL,
    email TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE posts (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    content TEXT,
    published BOOLEAN DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users (id)
);
EOF

# Update the configuration
cat > axel.yaml <<EOF
database:
  type: sqlite
  database: ./test.db

generators:
  - language: go
    options:
      package: models
  - language: python
  - language: typescript
  - language: javascript

output:
  directory: ./generated
EOF

# Generate code
axel introspect
axel generate
```

### PostgreSQL Example

```bash
# Connect to your PostgreSQL database and create tables
psql -U postgres -d mydb <<EOF
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    email VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE posts (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    title VARCHAR(200) NOT NULL,
    content TEXT,
    published BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
EOF

# Generate code
axel introspect
axel generate
```

## Generated Code Structure

After running `axel generate`, you'll find the generated code organized by language:

```
generated/
├── go/
│   ├── users.go
│   └── posts.go
├── python/
│   ├── __init__.py
│   ├── users.py
│   └── posts.py
├── typescript/
│   ├── index.ts
│   ├── users.ts
│   └── posts.ts
└── javascript/
    ├── index.js
    ├── users.js
    └── posts.js
```
