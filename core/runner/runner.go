package runner

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

// Runner executes AQL queries at runtime against a live database.
type Runner struct {
	db     *pgxpool.Pool
	schema *asl.SchemaIR
}

// New returns a Runner bound to a pgx pool and schema.
func New(db *pgxpool.Pool, schema *asl.SchemaIR) *Runner {
	return &Runner{db: db, schema: schema}
}

// Row is a single result row as a key-value map.
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

	// DELETE (and any statement without RETURNING) → command tag only.
	if stmt.Delete != nil {
		tag, err := r.db.Exec(ctx, compiled.SQL, args...)
		if err != nil {
			return nil, err
		}
		return &Result{RowsAffected: tag.RowsAffected()}, nil
	}

	rows, err := r.db.Query(ctx, compiled.SQL, args...)
	if err != nil {
		return nil, err
	}
	maps, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return nil, err
	}
	result := make([]Row, len(maps))
	for i, m := range maps {
		result[i] = Row(m)
	}
	return &Result{Rows: result}, nil
}

// buildArgs maps named params to the positional order required by compiled SQL.
func buildArgs(paramInfos []compiler.ParamInfo, named map[string]any) []any {
	args := make([]any, len(paramInfos))
	for i, p := range paramInfos {
		args[i] = named[p.Name]
	}
	return args
}
