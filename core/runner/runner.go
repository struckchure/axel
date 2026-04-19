package runner

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

// Runner executes AQL queries at runtime against a live database.
type Runner struct {
	db     *sql.DB
	schema *asl.SchemaIR
}

// New returns a Runner bound to db and schema.
func New(db *sql.DB, schema *asl.SchemaIR) *Runner {
	return &Runner{db: db, schema: schema}
}

// Row is a single result row as an ordered key-value map.
type Row map[string]any

// Result holds the outcome of a Run call.
type Result struct {
	// Rows contains returned rows (SELECT, INSERT/UPDATE with RETURNING).
	Rows []Row
	// RowsAffected is set for DELETE (and INSERT/UPDATE without RETURNING).
	RowsAffected int64
}

// Run compiles aqlQuery, binds params by name, executes against the database,
// and returns the result.
func (r *Runner) Run(ctx context.Context, aqlQuery string, params map[string]any) (*Result, error) {
	stmt, err := aql.ParseString(aqlQuery)
	if err != nil {
		return nil, fmt.Errorf("axel/runner: parse: %w", err)
	}

	compiled, err := compiler.Compile(stmt, r.schema)
	if err != nil {
		return nil, fmt.Errorf("axel/runner: compile: %w", err)
	}

	args := buildArgs(compiled.Params, params)

	switch {
	case stmt.Select != nil:
		return r.queryRows(ctx, compiled.SQL, args)
	case stmt.Delete != nil:
		return r.exec(ctx, compiled.SQL, args)
	default:
		return r.queryRows(ctx, compiled.SQL, args)
	}
}

// RunTx is like Run but executes within a caller-managed transaction.
func (r *Runner) RunTx(ctx context.Context, tx *sql.Tx, aqlQuery string, params map[string]any) (*Result, error) {
	stmt, err := aql.ParseString(aqlQuery)
	if err != nil {
		return nil, fmt.Errorf("axel/runner: parse: %w", err)
	}

	compiled, err := compiler.Compile(stmt, r.schema)
	if err != nil {
		return nil, fmt.Errorf("axel/runner: compile: %w", err)
	}

	args := buildArgs(compiled.Params, params)

	switch {
	case stmt.Select != nil:
		return queryRowsTx(ctx, tx, compiled.SQL, args)
	case stmt.Delete != nil:
		return execTx(ctx, tx, compiled.SQL, args)
	default:
		return queryRowsTx(ctx, tx, compiled.SQL, args)
	}
}

// buildArgs maps named params to the positional order required by compiled SQL.
func buildArgs(paramInfos []compiler.ParamInfo, named map[string]any) []any {
	args := make([]any, len(paramInfos))
	for i, p := range paramInfos {
		args[i] = named[p.Name]
	}
	return args
}

func (r *Runner) queryRows(ctx context.Context, query string, args []any) (*Result, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (r *Runner) exec(ctx context.Context, query string, args []any) (*Result, error) {
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	return &Result{RowsAffected: n}, nil
}

func queryRowsTx(ctx context.Context, tx *sql.Tx, query string, args []any) (*Result, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func execTx(ctx context.Context, tx *sql.Tx, query string, args []any) (*Result, error) {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	return &Result{RowsAffected: n}, nil
}

func scanRows(rows *sql.Rows) (*Result, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []Row
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(Row, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &Result{Rows: result}, nil
}
