package db

import "context"

// Column describes a single column of a table.
type Column struct {
	Name         string
	DataType     string // display type: postgres type, or ASL type when schema-driven
	SQLType      string // underlying SQL type (schema-driven only)
	Nullable     bool
	Default      string
	IsPrimaryKey bool
	IsForeignKey bool
	ForeignRef   string // "table.column" when IsForeignKey
	IsLink       bool   // ASL link (schema-driven)
	IsMulti      bool   // multi (junction) link — excluded from the row grid
}

// Table describes a table within a schema.
type Table struct {
	Schema  string
	Name    string
	Type    string // ASL type name when schema-driven, else ""
	Columns []Column
	Rows    int64 // estimated row count
}

// PrimaryKeys returns the names of the primary-key columns.
func (t Table) PrimaryKeys() []string {
	var pks []string
	for _, c := range t.Columns {
		if c.IsPrimaryKey {
			pks = append(pks, c.Name)
		}
	}
	return pks
}

// Rows is the result of a paginated table read.
type Rows struct {
	Columns []Column
	Values  [][]any // Values[row][col]
	Total   int64
	Offset  int
	Limit   int
}

// QueryResult is the result of an ad-hoc SQL or AQL query.
type QueryResult struct {
	Columns []string
	Rows    [][]any
	Command string // e.g. "SELECT 42", "INSERT 0 1", "compiled"
	Elapsed string
	Err     string
	SQL     string // compiled SQL (AQL console), shown for transparency
}

// Order describes sorting for a table read.
type Order struct {
	Column string
	Desc   bool
}

// Store is the data source backing the studio. It is implemented by a live
// Postgres connection and by an in-memory mock used when no database is
// reachable, so the UI always has something to render.
type Store interface {
	// Name is a short human label for the connection (db name or "sample data").
	Name() string
	// Live reports whether this store is backed by a real database.
	Live() bool
	// Tables lists every table grouped by schema, ordered by schema then name.
	Tables(ctx context.Context) ([]Table, error)
	// Table returns a single table's full metadata.
	Table(ctx context.Context, schema, name string) (Table, error)
	// Read returns a page of rows for a table.
	Read(ctx context.Context, schema, name string, limit, offset int, order *Order) (Rows, error)
	// Query runs an arbitrary read-only SQL statement.
	Query(ctx context.Context, sql string) QueryResult
}
