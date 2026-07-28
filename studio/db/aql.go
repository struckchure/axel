package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/lo"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/compiler"
	"github.com/struckchure/axel/core/runner"
)

// AQL executes AQL against a schema. With a live pool it runs queries; without
// one it still parses and compiles them to SQL for validation and preview.
type AQL struct {
	schema *Schema
	pool   *pgxpool.Pool  // nil when offline
	run    *runner.Runner // nil when offline
}

// NewAQL builds an AQL engine. pool may be nil, in which case the engine is
// compile-only (parses and lowers AQL to SQL but does not execute).
func NewAQL(schema *Schema, pool *pgxpool.Pool) *AQL {
	a := &AQL{schema: schema, pool: pool}
	if pool != nil {
		a.run = runner.New(pool, schema.IR)
	}
	return a
}

// Live reports whether queries actually execute against a database.
func (a *AQL) Live() bool { return a.run != nil }

// Exec runs an ad-hoc AQL statement for the console. Offline, it returns the
// compiled SQL instead of executing.
func (a *AQL) Exec(ctx context.Context, query string) QueryResult {
	start := time.Now()

	stmt, err := aql.ParseString(query)
	if err != nil {
		return QueryResult{Err: "parse: " + err.Error()}
	}
	compiled, err := compiler.Compile(stmt, a.schema.IR)
	if err != nil {
		return QueryResult{Err: "compile: " + err.Error()}
	}

	if !a.Live() {
		return QueryResult{
			Command: "compiled — not executed (offline)",
			SQL:     compiled.SQL,
		}
	}

	res, err := a.run.Run(ctx, query, nil)
	if err != nil {
		return QueryResult{Err: err.Error(), SQL: compiled.SQL, Elapsed: since(start)}
	}
	cols, values := rowsToGrid(res.Rows)
	cmd := fmt.Sprintf("%d rows", len(res.Rows))
	if res.Rows == nil {
		cmd = fmt.Sprintf("%d affected", res.RowsAffected)
	}
	return QueryResult{Columns: cols, Rows: values, Command: cmd, SQL: compiled.SQL, Elapsed: since(start)}
}

// Mutation is one staged row change from the editor.
type Mutation struct {
	Op    string              `json:"op"`    // "insert" | "update" | "delete"
	Type  string              `json:"type"`  // ASL type name
	ID    string              `json:"id"`    // primary key, for update/delete
	Set   map[string]string   `json:"set"`   // scalar + single-link column → value
	Multi map[string][]string `json:"multi"` // multi-link field → selected target ids
}

// MutateResult reports the outcome of applying a batch of mutations.
type MutateResult struct {
	Applied int
	Err     string
}

// Mutate applies a batch of staged row changes as AQL insert/update/delete
// statements, compiled and executed inline through the runtime runner. Values
// are bound as parameters (never string-interpolated) and coerced to the
// column's declared type. Statements run in order; on the first failure it
// stops and reports how many succeeded.
func (a *AQL) Mutate(ctx context.Context, muts []Mutation) MutateResult {
	if !a.Live() {
		return MutateResult{Err: "AQL is not connected to a live database"}
	}
	for i, m := range muts {
		if err := a.applyMutation(ctx, m); err != nil {
			return MutateResult{Applied: i, Err: err.Error()}
		}
	}
	return MutateResult{Applied: len(muts)}
}

// applyMutation runs one change: scalar/single-link fields go through AQL, and
// multi-link fields are reconciled against the junction table afterwards (the
// AQL compiler does not write multi links).
func (a *AQL) applyMutation(ctx context.Context, m Mutation) error {
	switch m.Op {
	case "delete":
		if m.ID == "" {
			return fmt.Errorf("delete: missing id")
		}
		_, err := a.run.Run(ctx, fmt.Sprintf("delete %s filter .id = $__id;", m.Type),
			map[string]any{"__id": m.ID})
		return err

	case "insert":
		assigns, params, err := a.assignments(m.Type, m.Set, true)
		if err != nil {
			return err
		}
		if len(assigns) == 0 {
			return fmt.Errorf("insert: no values provided")
		}
		res, err := a.run.Run(ctx, fmt.Sprintf("insert %s { %s };", m.Type, strings.Join(assigns, ", ")), params)
		if err != nil {
			return err
		}
		id := m.ID
		if len(res.Rows) > 0 {
			id = idString(res.Rows[0]["id"])
		}
		return a.applyMultiLinks(ctx, m.Type, id, m.Multi)

	case "update":
		if m.ID == "" {
			return fmt.Errorf("update: missing id")
		}
		assigns, params, err := a.assignments(m.Type, m.Set, false)
		if err != nil {
			return err
		}
		if len(assigns) > 0 {
			params["__id"] = m.ID
			if _, err := a.run.Run(ctx, fmt.Sprintf("update %s filter .id = $__id set { %s };", m.Type, strings.Join(assigns, ", ")), params); err != nil {
				return err
			}
		}
		return a.applyMultiLinks(ctx, m.Type, m.ID, m.Multi)

	default:
		return fmt.Errorf("unknown op %q", m.Op)
	}
}

// applyMultiLinks reconciles every changed multi link on a row.
func (a *AQL) applyMultiLinks(ctx context.Context, ownerType, ownerID string, multi map[string][]string) error {
	if len(multi) == 0 {
		return nil
	}
	if ownerID == "" {
		return fmt.Errorf("cannot set links: row has no id")
	}
	for field, ids := range multi {
		target, ok := a.schema.MultiLinkTarget(ownerType, field)
		if !ok {
			continue
		}
		if err := a.setMultiLink(ctx, ownerType, ownerID, field, target, ids); err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
	}
	return nil
}

// setMultiLink replaces a row's membership in a multi link's junction table with
// the given set of target ids, in one transaction. Junction naming follows
// axel's convention: table "<owner>_<link>", columns "<owner>" and "<target>"
// (all snake-cased).
func (a *AQL) setMultiLink(ctx context.Context, ownerType, ownerID, field, targetType string, ids []string) error {
	junction := pgIdent(lo.SnakeCase(ownerType) + "_" + lo.SnakeCase(field))
	ownerCol := pgIdent(lo.SnakeCase(ownerType))
	targetCol := pgIdent(lo.SnakeCase(targetType))

	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = $1", junction, ownerCol), ownerID); err != nil {
		return err
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, err := tx.Exec(ctx,
			fmt.Sprintf("INSERT INTO %s (%s, %s) VALUES ($1, $2) ON CONFLICT DO NOTHING", junction, ownerCol, targetCol),
			ownerID, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// assignments turns raw column values into AQL assignment fragments plus a
// params map. Scalars become `col := $col`; single links resolve their target
// row by id via `col := (select Target filter .id = $col)`. Multi links are
// skipped (the compiler does not support multi-link writes). On insert, empty
// values are skipped so column defaults apply; on update they become NULLs.
func (a *AQL) assignments(typeName string, set map[string]string, isInsert bool) ([]string, map[string]any, error) {
	assigns := make([]string, 0, len(set))
	params := make(map[string]any, len(set))
	for col, raw := range set {
		// Single link → resolve the referenced row by id.
		if target, ok := a.schema.SingleLinkTarget(typeName, col); ok {
			if raw == "" && isInsert {
				continue
			}
			assigns = append(assigns, fmt.Sprintf("%s := (select %s filter .id = $%s)", col, target, col))
			if raw == "" {
				params[col] = nil
			} else {
				params[col] = raw
			}
			continue
		}
		// Multi link → not writable via AQL yet.
		if a.schema.IsMultiLink(typeName, col) {
			continue
		}

		aqlType, nullable, ok := a.schema.ColumnType(typeName, col)
		if !ok {
			continue // unknown field
		}
		if raw == "" && isInsert {
			continue
		}
		val, err := coerce(aqlType, raw, nullable)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", col, err)
		}
		assigns = append(assigns, fmt.Sprintf("%s := $%s", col, col))
		params[col] = val
	}
	return assigns, params, nil
}

// Candidates returns up to 200 selectable rows of a target type for a link
// picker, each with an id and a human-readable label. Live only.
func (a *AQL) Candidates(ctx context.Context, target string) ([]Option, error) {
	if !a.Live() {
		return nil, nil
	}
	label := a.schema.labelField(target)
	shape := "id"
	if label != "" && label != "id" {
		shape = "id, " + label
	}
	res, err := a.run.Run(ctx, fmt.Sprintf("multi select %s { %s } limit 200;", target, shape), nil)
	if err != nil {
		return nil, err
	}
	opts := make([]Option, 0, len(res.Rows))
	for _, row := range res.Rows {
		id := idString(row["id"])
		lab := id
		if label != "id" {
			if lv, ok := row[label]; ok && lv != nil {
				lab = fmt.Sprintf("%v", lv)
			}
		}
		opts = append(opts, Option{ID: id, Label: lab})
	}
	return opts, nil
}

// idString renders a primary-key value as a string, formatting pgx's [16]byte
// uuid as a canonical uuid.
func idString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case [16]byte:
		return fmt.Sprintf("%x-%x-%x-%x-%x", x[0:4], x[4:6], x[6:8], x[8:10], x[10:16])
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

// coerce converts a raw editor string into a Go value matching the AQL type.
// An empty string becomes NULL.
func coerce(aqlType, raw string, _ bool) (any, error) {
	if raw == "" {
		return nil, nil
	}
	switch aqlType {
	case "int16", "int32", "int64", "int":
		return strconv.ParseInt(raw, 10, 64)
	case "float32", "float64", "float", "decimal":
		return strconv.ParseFloat(raw, 64)
	case "bool":
		return strconv.ParseBool(raw)
	default:
		return raw, nil // str, uuid, datetime, json, enums — pg casts from text
	}
}

// Read fetches a page of a table's rows via an AQL select. Live only.
func (a *AQL) Read(ctx context.Context, t Table, limit, offset int, order *Order) (Rows, error) {
	cols := gridColumns(t)

	total, err := a.count(ctx, t.Type)
	if err != nil {
		return Rows{}, err
	}

	query := buildReadAQL(t.Type, cols, limit, offset, order)
	res, err := a.run.Run(ctx, query, nil)
	if err != nil {
		return Rows{}, fmt.Errorf("%w\n\n%s", err, query)
	}

	values := make([][]any, len(res.Rows))
	for i, row := range res.Rows {
		vals := make([]any, len(cols))
		for j, c := range cols {
			vals[j] = flattenLink(row[c.Name], c)
		}
		values[i] = vals
	}
	return Rows{Columns: cols, Values: values, Total: total, Offset: offset, Limit: limit}, nil
}

// buildReadAQL assembles `select Type { scalars…, link: { id } } order by … limit … offset …`.
func buildReadAQL(typeName string, cols []Column, limit, offset int, order *Order) string {
	var fields []string
	pk := ""
	for _, c := range cols {
		if c.IsLink {
			fields = append(fields, c.Name+": { id }")
			continue
		}
		fields = append(fields, c.Name)
		if c.IsPrimaryKey && pk == "" {
			pk = c.Name
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "multi select %s {\n  %s\n}", typeName, strings.Join(fields, ",\n  "))

	orderCol, dir := pk, "asc"
	if order != nil && order.Column != "" {
		orderCol = order.Column
		if order.Desc {
			dir = "desc"
		}
	}
	if orderCol != "" {
		fmt.Fprintf(&b, " order by .%s %s", orderCol, dir)
	}
	fmt.Fprintf(&b, " limit %d offset %d", limit, offset)
	return b.String()
}

func (a *AQL) count(ctx context.Context, typeName string) (int64, error) {
	res, err := a.run.Run(ctx, fmt.Sprintf("select count(%s);", typeName), nil)
	if err != nil || len(res.Rows) == 0 {
		return 0, err
	}
	for _, v := range res.Rows[0] {
		return toInt64(v), nil
	}
	return 0, nil
}

// flattenLink shapes a selected link value for grid display. A single link's
// sub-object ({id: ...}) collapses to its id; a multi link's JSON array is left
// intact and rendered as an array of ids.
func flattenLink(v any, c Column) any {
	if !c.IsLink {
		return v
	}
	if c.IsMulti {
		return multiLinkIDs(v) // []any of {id: ...} → []any of ids
	}
	if m, ok := v.(map[string]any); ok {
		if id, ok := m["id"]; ok {
			return id
		}
	}
	return v
}

// multiLinkIDs reduces a multi link's [{id: ...}, ...] to a slice of ids.
func multiLinkIDs(v any) any {
	arr, ok := v.([]any)
	if !ok {
		return v
	}
	ids := make([]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			if id, ok := m["id"]; ok {
				ids = append(ids, id)
				continue
			}
		}
		ids = append(ids, item)
	}
	return ids
}

func rowsToGrid(rows []runner.Row) ([]string, [][]any) {
	if len(rows) == 0 {
		return nil, nil
	}
	var cols []string
	for k := range rows[0] {
		cols = append(cols, k)
	}
	// stable column order
	sortStrings(cols)
	values := make([][]any, len(rows))
	for i, r := range rows {
		vals := make([]any, len(cols))
		for j, c := range cols {
			vals[j] = r[c]
		}
		values[i] = vals
	}
	return cols, values
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

func since(t time.Time) string { return time.Since(t).Round(time.Microsecond).String() }

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
