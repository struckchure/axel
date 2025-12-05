package db

import (
	"database/sql"
	"fmt"

	"github.com/struckchure/axel/pkg/config"
	_ "github.com/lib/pq"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

// Connection represents a database connection
type Connection struct {
	db       *sql.DB
	dbType   string
	dbConfig config.DatabaseConfig
}

// Schema represents a database schema
type Schema struct {
	Tables []Table
}

// Table represents a database table
type Table struct {
	Name    string
	Columns []Column
}

// Column represents a table column
type Column struct {
	Name     string
	Type     string
	Nullable bool
	IsPrimaryKey bool
}

// Connect creates a new database connection
func Connect(cfg config.DatabaseConfig) (*Connection, error) {
	var dsn string

	switch cfg.Type {
	case "postgres":
		dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database, cfg.SSLMode)
	case "mysql":
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	case "sqlite":
		dsn = cfg.Database
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	// Map database type to driver name
	driverName := cfg.Type
	if cfg.Type == "sqlite" {
		driverName = "sqlite3"
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Connection{
		db:       db,
		dbType:   cfg.Type,
		dbConfig: cfg,
	}, nil
}

// Close closes the database connection
func (c *Connection) Close() error {
	return c.db.Close()
}

// IntrospectSchema introspects the database schema
func (c *Connection) IntrospectSchema() (*Schema, error) {
	switch c.dbType {
	case "postgres":
		return c.introspectPostgres()
	case "mysql":
		return c.introspectMySQL()
	case "sqlite":
		return c.introspectSQLite()
	default:
		return nil, fmt.Errorf("unsupported database type: %s", c.dbType)
	}
}

func (c *Connection) introspectPostgres() (*Schema, error) {
	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []Table
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}

		columns, err := c.introspectPostgresTable(tableName)
		if err != nil {
			return nil, err
		}

		tables = append(tables, Table{
			Name:    tableName,
			Columns: columns,
		})
	}

	return &Schema{Tables: tables}, nil
}

func (c *Connection) introspectPostgresTable(tableName string) ([]Column, error) {
	query := `
		SELECT 
			c.column_name,
			c.data_type,
			c.is_nullable,
			CASE WHEN pk.column_name IS NOT NULL THEN true ELSE false END as is_primary_key
		FROM information_schema.columns c
		LEFT JOIN (
			SELECT ku.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage ku
				ON tc.constraint_name = ku.constraint_name
			WHERE tc.constraint_type = 'PRIMARY KEY'
				AND tc.table_name = $1
		) pk ON c.column_name = pk.column_name
		WHERE c.table_name = $1
		ORDER BY c.ordinal_position
	`

	rows, err := c.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %w", err)
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var col Column
		var nullable string
		if err := rows.Scan(&col.Name, &col.Type, &nullable, &col.IsPrimaryKey); err != nil {
			return nil, err
		}
		col.Nullable = (nullable == "YES")
		columns = append(columns, col)
	}

	return columns, nil
}

func (c *Connection) introspectMySQL() (*Schema, error) {
	query := fmt.Sprintf("SHOW TABLES FROM %s", c.dbConfig.Database)

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []Table
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}

		columns, err := c.introspectMySQLTable(tableName)
		if err != nil {
			return nil, err
		}

		tables = append(tables, Table{
			Name:    tableName,
			Columns: columns,
		})
	}

	return &Schema{Tables: tables}, nil
}

func (c *Connection) introspectMySQLTable(tableName string) ([]Column, error) {
	query := fmt.Sprintf("SHOW COLUMNS FROM %s", tableName)

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %w", err)
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var col Column
		var nullable, key, extra string
		var defaultVal sql.NullString

		if err := rows.Scan(&col.Name, &col.Type, &nullable, &key, &defaultVal, &extra); err != nil {
			return nil, err
		}

		col.Nullable = (nullable == "YES")
		col.IsPrimaryKey = (key == "PRI")
		columns = append(columns, col)
	}

	return columns, nil
}

func (c *Connection) introspectSQLite() (*Schema, error) {
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []Table
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}

		columns, err := c.introspectSQLiteTable(tableName)
		if err != nil {
			return nil, err
		}

		tables = append(tables, Table{
			Name:    tableName,
			Columns: columns,
		})
	}

	return &Schema{Tables: tables}, nil
}

func (c *Connection) introspectSQLiteTable(tableName string) ([]Column, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %w", err)
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var col Column
		var cid, notNull, pk int
		var defaultVal sql.NullString

		if err := rows.Scan(&cid, &col.Name, &col.Type, &notNull, &defaultVal, &pk); err != nil {
			return nil, err
		}

		col.Nullable = (notNull == 0)
		col.IsPrimaryKey = (pk > 0)
		columns = append(columns, col)
	}

	return columns, nil
}
