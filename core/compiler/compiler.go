package compiler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

// sortedProps returns a type's scalar properties ordered by property name.
// This MUST match codegen's allPropsAsFields (core/codegen/descriptor.go) so
// that the compiled SQL's columns line up with the generated struct's fields.
// Single-link FK columns are appended after the scalar properties by
// sortedSingleLinks; keep all three in lockstep.
func sortedProps(rt *asl.ResolvedType) []*asl.ResolvedProp {
	names := make([]string, 0, len(rt.Properties))
	for n := range rt.Properties {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*asl.ResolvedProp, len(names))
	for i, n := range names {
		out[i] = rt.Properties[n]
	}
	return out
}

// sortedSingleLinks returns a type's single (non-multi) links ordered by link
// name. Their FK columns are part of "all columns" for `select *` / RETURNING
// so reference fields are not omitted from the row. Multi-links live in
// junction tables and have no FK column here, so they are excluded.
func sortedSingleLinks(rt *asl.ResolvedType) []*asl.ResolvedLink {
	names := make([]string, 0, len(rt.Links))
	for n, l := range rt.Links {
		if l.IsMulti {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*asl.ResolvedLink, len(names))
	for i, n := range names {
		out[i] = rt.Links[n]
	}
	return out
}

// returningColumns builds an explicit RETURNING column list (quoted) covering
// scalar properties followed by single-link FK columns, so the result columns
// match the generated row struct.
func returningColumns(rt *asl.ResolvedType) string {
	var cols []string
	for _, p := range sortedProps(rt) {
		cols = append(cols, fmt.Sprintf("%q", p.Column))
	}
	for _, l := range sortedSingleLinks(rt) {
		cols = append(cols, fmt.Sprintf("%q", l.JoinColumn))
	}
	return strings.Join(cols, ", ")
}

// CompileOptions configures query compilation.
type CompileOptions struct {
	RelLoadStrategy string // "query" (default, correlated subqueries) or "join" (LEFT JOIN LATERAL)
}

// Compile compiles a parsed AQL statement against a SchemaIR into SQL using default options.
func Compile(stmt *aql.Statement, schema *asl.SchemaIR) (*CompiledSQL, error) {
	return CompileWithOptions(stmt, schema, CompileOptions{})
}

// CompileWithOptions compiles a parsed AQL statement against a SchemaIR with custom options.
func CompileWithOptions(stmt *aql.Statement, schema *asl.SchemaIR, opts CompileOptions) (*CompiledSQL, error) {
	strategy := opts.RelLoadStrategy
	if stmt != nil {
		if v, ok := stmt.DirectiveMap()["rel_load_strategy"]; ok && v != "" {
			strategy = v
		}
	}
	if strategy == "" {
		strategy = "query"
	}
	strategy = strings.ToLower(strategy)
	if strategy != "query" && strategy != "join" {
		return nil, fmt.Errorf("invalid rel_load_strategy %q: must be 'query' or 'join'", strategy)
	}

	c := &compiler{
		schema:          schema,
		params:          newParamCollector(),
		relLoadStrategy: strategy,
	}

	// Variables are declared first so they receive initial positional slots
	// and explicit types/optionality ahead of use sites.
	if err := c.compileVars(stmt.Vars); err != nil {
		return nil, err
	}

	// Bindings are lowered next so their params are numbered in the order the
	// CTEs are emitted, ahead of the statement body that references them.
	if err := c.compileWith(stmt.With); err != nil {
		return nil, err
	}

	var sql string
	var err error

	switch {
	case stmt.Select != nil:
		sql, err = c.compileSelect(stmt.Select)
	case stmt.Insert != nil:
		// compileInsertBody emits its own WITH — it may add sub-insert CTEs of
		// its own — so it folds the bindings in rather than being prefixed here.
		sql, err = c.compileInsert(stmt.Insert)
	case stmt.Update != nil:
		sql, err = c.compileUpdate(stmt.Update)
	case stmt.Delete != nil:
		sql, err = c.compileDelete(stmt.Delete)
	default:
		return nil, fmt.Errorf("empty statement")
	}
	if err != nil {
		return nil, err
	}
	if stmt.Insert == nil {
		sql = c.withPrefix() + sql
	}

	return &CompiledSQL{
		SQL:    sql,
		Params: c.params.params,
	}, nil
}

type compiler struct {
	schema          *asl.SchemaIR
	params          *paramCollector
	relLoadStrategy string
	// aliasCounts tracks how many times each base alias letter has been handed
	// out within this compile so every table instance gets a distinct alias: the
	// first use of a letter keeps the bare form ("w"), later uses are suffixed
	// ("w1", "w2", …). Correlated subqueries thread their owner's alias down as
	// the `alias` parameter, so guaranteeing uniqueness here is enough to stop an
	// inner table from shadowing an outer one — the classic FILTER-traversal
	// collision where WorkflowTrigger and Workflow both derived alias "w". See
	// newAlias and compilePath.
	aliasCounts map[string]int
	// trig is non-nil when compiling a trigger / function body, enabling the
	// magic identifiers __new__ / __old__ / __subject__ / event.
	trig *triggerContext
	// valueFilter is true while compiling the filter of a scalar subquery used
	// as a value (a link assignment or `(select ...)` operand). There a lone
	// omitted optional param must yield no row — NULL, which composes with `??`
	// — instead of matching all rows and returning an arbitrary one. See
	// compileValueFilter / compileCmp.
	valueFilter bool
	// withs maps a `with (...)` binding name to the CTE backing it. Bindings
	// shadow type names of the same spelling; see compileWith / withRef.
	withs map[string]*withBinding
	// withCTEs holds the binding CTEs in declaration order, emitted as the
	// statement's WITH prefix.
	withCTEs []string
	// policyMode is true while lowering an RLS policy predicate: the target row's
	// columns are emitted unqualified and bind params are rejected. Link traversal
	// is allowed and correlates back to the base row by table name (see outerRef).
	// See CompilePolicyPredicate.
	policyMode bool
}

// CompilePolicyPredicate lowers an AQL boolean expression to a Postgres RLS policy
// predicate (the interior of USING / WITH CHECK). Unlike query compilation it runs
// in "policy mode": the target row's own columns are emitted unqualified (RLS
// predicates run with those columns in scope) and bind params ($x) are rejected (a
// policy can't take parameters — reference a `global` instead). Link traversal is
// supported: to-one chains (.organization.owner) lower to a correlated subquery,
// and `<value> in .<multi-link>` lowers to a membership subquery over the junction.
// `global <name>` and `is [not] null` are also supported.
func CompilePolicyPredicate(expr *aql.Expr, rt *asl.ResolvedType, schema *asl.SchemaIR) (string, error) {
	c := &compiler{schema: schema, params: newParamCollector(), policyMode: true}
	return c.compileExpr(expr, "", rt)
}

// triggerContext carries the state that changes AQL compilation inside a trigger
// or function body.
type triggerContext struct {
	enclosing *asl.ResolvedType // for __new__/__old__ field validation; nil in a standalone function
	params    map[string]bool   // declared function parameter names (referenced as $name)
}

// ─────────────────────────────────────────────────────────────
// SELECT
// ─────────────────────────────────────────────────────────────

func (c *compiler) compileSelect(stmt *aql.SelectStmt) (string, error) {
	body := stmt.Body

	// Aggregate select: select count(TypeName filter expr)
	if body.AggFunc != nil {
		return c.compileAgg(body.AggFunc)
	}

	typeName := body.TypeName
	rt, err := c.resolveType(typeName)
	if err != nil {
		return "", err
	}

	alias := c.newAlias(typeName)
	table := rt.Table

	// Aggregation select: a shape whose fields are aggregates over the scanned
	// set (each with an optional per-field `filter`). Lowers to a single scan with
	// `FUNC(arg) FILTER (WHERE ...)` columns and the shared filter as the outer
	// WHERE — one row out, no LIMIT.
	if body.Shape != nil && shapeIsAggregation(body.Shape) && len(body.GroupBy) == 0 {
		return c.compileAggregationSelect(stmt, rt, alias)
	}

	if len(body.GroupBy) > 0 {
		if err := c.validateGroupByShape(body.Shape, body.GroupBy, rt); err != nil {
			return "", err
		}
	}

	// Build SELECT columns from shape (or "*" if no shape).
	var cols []string
	var laterals []string

	if body.Shape != nil {
		cols, laterals, err = c.compileShapeCols(body.Shape, rt, alias)
		if err != nil {
			return "", err
		}
	} else {
		// No shape → select all scalar properties plus single-link FK columns,
		// so `select *` returns reference fields too (see sortedProps /
		// sortedSingleLinks and codegen's allPropsAsFields).
		for _, prop := range sortedProps(rt) {
			cols = append(cols, fmt.Sprintf("%s.%s", alias, prop.Column))
		}
		for _, link := range sortedSingleLinks(rt) {
			cols = append(cols, fmt.Sprintf("%s.%s", alias, link.JoinColumn))
		}
	}

	var sb strings.Builder
	sb.WriteString("SELECT\n  ")
	sb.WriteString(strings.Join(cols, ",\n  "))
	fmt.Fprintf(&sb, "\nFROM \"%s\" %s", table, alias)

	// Append lateral subqueries (for nested link shapes).
	for _, lat := range laterals {
		sb.WriteString("\n")
		sb.WriteString(lat)
	}

	// WHERE
	if body.Filter != nil {
		where, err := c.compileExpr(body.Filter.Expr, alias, rt)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "\nWHERE %s", where)
	}

	// GROUP BY
	if len(body.GroupBy) > 0 {
		var parts []string
		for _, g := range body.GroupBy {
			expr, err := c.compileExpr(g.Expr, alias, rt)
			if err != nil {
				return "", err
			}
			parts = append(parts, expr)
		}
		fmt.Fprintf(&sb, "\nGROUP BY %s", strings.Join(parts, ", "))
	}

	// HAVING
	if body.Having != nil {
		if len(body.GroupBy) == 0 {
			return "", fmt.Errorf("HAVING clause requires a GROUP BY clause")
		}
		having, err := c.compileExpr(body.Having.Expr, alias, rt)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "\nHAVING %s", having)
	}

	// ORDER BY
	if len(body.OrderBy) > 0 {
		var parts []string
		for _, o := range body.OrderBy {
			expr, err := c.compileExpr(o.Expr, alias, rt)
			if err != nil {
				return "", err
			}
			dir := strings.ToUpper(o.Dir)
			if dir == "" {
				dir = "ASC"
			}
			parts = append(parts, expr+" "+dir)
		}
		fmt.Fprintf(&sb, "\nORDER BY %s", strings.Join(parts, ", "))
	}

	// LIMIT / OFFSET.
	// A plain select returns a single row (implicit LIMIT 1). `multi select`
	// returns all rows and honours explicit limit/offset.
	if stmt.Multi {
		if body.Limit != nil {
			limit, err := c.compileExpr(body.Limit, alias, rt)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&sb, "\nLIMIT %s", limit)
		}
		if body.Offset != nil {
			offset, err := c.compileExpr(body.Offset, alias, rt)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&sb, "\nOFFSET %s", offset)
		}
	} else {
		if body.Limit != nil || body.Offset != nil {
			return "", fmt.Errorf("limit/offset require 'multi select' (a plain select returns a single row)")
		}
		sb.WriteString("\nLIMIT 1")
	}

	sb.WriteString(";")
	return sb.String(), nil
}

func (c *compiler) compileAgg(agg *aql.AggExpr) (string, error) {
	sql, err := c.compileAggSQL(agg)
	if err != nil {
		return "", err
	}
	return sql + ";", nil
}

// compileAggSQL builds the SELECT for an aggregate without a trailing
// terminator, so it can serve both as a top-level statement (compileAgg appends
// ";") and as a scalar subquery operand (compileSubQuery wraps it in parens).
func (c *compiler) compileAggSQL(agg *aql.AggExpr) (string, error) {
	rt, err := c.resolveType(agg.TypeName)
	if err != nil {
		return "", err
	}
	alias := c.newAlias(agg.TypeName)

	inner := fmt.Sprintf("SELECT 1 FROM \"%s\" %s", rt.Table, alias)
	if agg.Filter != nil {
		where, err := c.compileExpr(agg.Filter.Expr, alias, rt)
		if err != nil {
			return "", err
		}
		inner += "\n  WHERE " + where
	}

	switch strings.ToLower(agg.Func) {
	case "count":
		return fmt.Sprintf("SELECT COUNT(*) FROM (\n  %s\n) _agg", inner), nil
	default:
		// The top-level `select func(Type filter ...)` form aggregates over `*`,
		// which is only meaningful for count. sum/avg/min/max need a column — that
		// is the aggregation-select shape form, `select Type { t := sum(.col) }`.
		return "", fmt.Errorf("aggregate %q is not valid in `select %s(%s ...)`; only count aggregates over a whole type. For sum/avg/min/max, use an aggregate shape: select %s { name := %s(.column) filter ... }",
			agg.Func, agg.Func, agg.TypeName, agg.TypeName, strings.ToLower(agg.Func))
	}
}

// shapeIsAggregation reports whether a shape is an aggregation shape — at least
// one field is an aggregate value (sum/avg/min/max/count) or carries a per-field
// `filter` tail. Such a shape makes the enclosing select an aggregation select,
// where every field must be an aggregate (enforced in compileAggField).
func shapeIsAggregation(shape *aql.Shape) bool {
	for _, f := range shape.Fields {
		if _, _, ok := f.AggCall(); ok {
			return true
		}
		if f.AggFilter != nil {
			return true
		}
	}
	return false
}

// compileAggregationSelect lowers an aggregation select to a single-scan SQL
// statement: one `FUNC(arg) [FILTER (WHERE ...)] AS name` column per shape field,
// the shared `filter` as the outer WHERE, and no LIMIT (an ungrouped aggregate
// always yields exactly one row).
func (c *compiler) compileAggregationSelect(stmt *aql.SelectStmt, rt *asl.ResolvedType, alias string) (string, error) {
	body := stmt.Body
	if stmt.Multi {
		return "", fmt.Errorf("an aggregate select returns a single row; drop `multi`")
	}
	if len(body.OrderBy) > 0 || body.Limit != nil || body.Offset != nil {
		return "", fmt.Errorf("order by / limit / offset are not allowed on an aggregate select (it returns a single row)")
	}

	cols := make([]string, 0, len(body.Shape.Fields))
	for _, f := range body.Shape.Fields {
		col, err := c.compileAggField(f, rt, alias)
		if err != nil {
			return "", err
		}
		cols = append(cols, col)
	}

	var sb strings.Builder
	sb.WriteString("SELECT\n  ")
	sb.WriteString(strings.Join(cols, ",\n  "))
	fmt.Fprintf(&sb, "\nFROM \"%s\" %s", rt.Table, alias)

	if body.Filter != nil {
		where, err := c.compileExpr(body.Filter.Expr, alias, rt)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "\nWHERE %s", where)
	}

	sb.WriteString(";")
	return sb.String(), nil
}

// compileAggField compiles one aggregate shape field to a
// `FUNC(arg) [FILTER (WHERE cond)] [::cast] AS name` column. It enforces the
// all-aggregate rule: a non-aggregate field in an aggregation shape is an error.
func (c *compiler) compileAggField(f *aql.ShapeField, rt *asl.ResolvedType, alias string) (string, error) {
	fc, cast, ok := f.AggCall()
	if !ok {
		return "", fmt.Errorf("field %q: an aggregate select may only contain aggregate fields (sum/avg/min/max/count over .column); mix with row fields is not allowed", f.Name)
	}

	fn := strings.ToUpper(fc.Name)
	var argSQL string
	switch len(fc.Args) {
	case 0:
		if fn != "COUNT" {
			return "", fmt.Errorf("field %q: %s requires one argument, e.g. %s(.column)", f.Name, strings.ToLower(fn), strings.ToLower(fn))
		}
		argSQL = "*" // count() → COUNT(*)
	case 1:
		s, err := c.compileExpr(fc.Args[0], alias, rt)
		if err != nil {
			return "", err
		}
		argSQL = s
	default:
		return "", fmt.Errorf("field %q: aggregate %s takes a single argument", f.Name, strings.ToLower(fn))
	}

	aggSQL := fmt.Sprintf("%s(%s)", fn, argSQL)

	if f.AggFilter != nil {
		where, err := c.compileExpr(f.AggFilter.Expr, alias, rt)
		if err != nil {
			return "", err
		}
		aggSQL += fmt.Sprintf(" FILTER (WHERE %s)", where)
	}

	if cast != "" {
		sqlType, err := c.annotSQLType(cast)
		if err != nil {
			return "", err
		}
		aggSQL = fmt.Sprintf("(%s)::%s", aggSQL, sqlType)
	}

	return fmt.Sprintf("%s AS %s", aggSQL, f.Name), nil
}

// compileShapeCols compiles every field of a shape against a type/alias into a
// flat list of SELECT column expressions and any associated lateral JOIN clauses.
// `*` expands to scalar properties plus single-link FK columns (skipping any name
// selected explicitly elsewhere in the shape). Because it delegates non-star
// fields to compileShapeField, nested links, computed fields, and splats all work
// at any depth — the top-level shape and a link's sub-shape share this one code path.
func (c *compiler) compileShapeCols(shape *aql.Shape, rt *asl.ResolvedType, alias string) ([]string, []string, error) {
	// Collect explicitly-named fields so a `*` splat can skip them (explicit
	// selections win over the splat expansion).
	explicit := make(map[string]bool)
	for _, f := range shape.Fields {
		if !f.Star {
			explicit[f.Name] = true
		}
	}

	var cols []string
	var laterals []string
	for _, f := range shape.Fields {
		if f.Star {
			for _, prop := range sortedProps(rt) {
				if !explicit[prop.Name] {
					cols = append(cols, fmt.Sprintf("%s.%s AS %s", alias, prop.Column, prop.Name))
				}
			}
			for _, link := range sortedSingleLinks(rt) {
				if !explicit[link.Name] {
					cols = append(cols, fmt.Sprintf("%s.%s AS %s", alias, link.JoinColumn, link.JoinColumn))
				}
			}
			continue
		}
		col, lat, err := c.compileShapeField(f, rt, alias)
		if err != nil {
			return nil, nil, err
		}
		cols = append(cols, col)
		if lat != "" {
			laterals = append(laterals, lat)
		}
	}
	return cols, laterals, nil
}

func (c *compiler) validateGroupByShape(shape *aql.Shape, groupBy []*aql.GroupBy, rt *asl.ResolvedType) error {
	if shape == nil {
		return fmt.Errorf("grouped select requires an explicit shape")
	}

	groupedFields := make(map[string]bool)
	for _, g := range groupBy {
		if p := g.Expr.SoloPrimary(); p != nil && p.Path != nil && len(p.Path.Steps) > 0 {
			groupedFields[p.Path.Steps[0]] = true
		}
	}

	for _, f := range shape.Fields {
		if f.Star {
			return fmt.Errorf("wildcard '*' is not allowed in a grouped select; list grouping fields and aggregate fields explicitly")
		}
		if _, _, ok := f.AggCall(); ok {
			continue
		}
		if f.Computed != nil {
			continue
		}
		if !groupedFields[f.Name] {
			return fmt.Errorf("field %q must appear in the GROUP BY clause or be used in an aggregate function", f.Name)
		}
	}
	return nil
}

// compileShapeField compiles one field in a shape.
// Returns (column expression, lateral subquery string, error).
func (c *compiler) compileShapeField(f *aql.ShapeField, parentType *asl.ResolvedType, parentAlias string) (string, string, error) {
	// Aggregate field: name := sum(...) [filter ...] [<cast>]
	if _, _, ok := f.AggCall(); ok {
		col, err := c.compileAggField(f, parentType, parentAlias)
		return col, "", err
	}

	// Inline computed field: name := expr
	if f.Computed != nil {
		return c.compileComputedShapeField(f, parentType, parentAlias)
	}

	// Check computed properties.
	if comp, ok := parentType.Computed[f.Name]; ok {
		expr := expandComputedExpr(comp.Expr, parentAlias)
		return fmt.Sprintf("(%s) AS %s", expr, f.Name), "", nil
	}

	// Check scalar properties.
	if prop, ok := parentType.Properties[f.Name]; ok {
		col := fmt.Sprintf("%s.%s AS %s", parentAlias, prop.Column, f.Name)
		return col, "", nil
	}

	// Check links.
	if link, ok := parentType.Links[f.Name]; ok {
		return c.compileLinkField(f, link, parentType, parentAlias)
	}

	return "", "", fmt.Errorf("type %q has no field %q", parentType.Name, f.Name)
}

func (c *compiler) compileLinkField(f *aql.ShapeField, link *asl.ResolvedLink, parentType *asl.ResolvedType, parentAlias string) (string, string, error) {
	targetType, err := c.resolveType(link.TargetType)
	if err != nil {
		return "", "", err
	}
	tAlias := tableAlias(link.TargetType) + "_" + f.Name

	// Collect columns for the sub-shape (or all properties if no sub-shape).
	// A sub-shape goes through compileShapeCols so nested links, computed
	// fields, and `*` splats resolve just like a top-level shape — the nested
	// link's own correlated subquery becomes one column of this subquery's row.
	var subCols []string
	var subLaterals []string
	if f.SubShape != nil {
		subCols, subLaterals, err = c.compileShapeCols(f.SubShape, targetType, tAlias)
		if err != nil {
			return "", "", err
		}
	} else {
		for _, prop := range targetType.Properties {
			subCols = append(subCols, fmt.Sprintf("%s.%s", tAlias, prop.Column))
		}
	}

	var subLatClause string
	if len(subLaterals) > 0 {
		subLatClause = " " + strings.Join(subLaterals, " ")
	}

	if link.IsMulti {
		// Multi-link → a correlated json_agg scalar subquery or LEFT JOIN LATERAL.
		var inner string
		if link.JunctionTable != "" {
			jAlias := "jt_" + f.Name
			joinField := link.JoinField
			if joinField == "" {
				joinField = "id"
			}
			inner = fmt.Sprintf(
				"SELECT %s FROM \"%s\" %s JOIN \"%s\" %s ON %s.%s = %s.%s%s WHERE %s.%s = %s.id",
				strings.Join(subCols, ", "),
				link.JunctionTable, jAlias,
				targetType.Table, tAlias,
				tAlias, joinField, jAlias, targetType.Table,
				subLatClause,
				jAlias, parentType.Table, parentAlias,
			)
		} else {
			// Direct FK on the target side (rare for multi).
			inner = fmt.Sprintf(
				"SELECT %s FROM \"%s\" %s%s WHERE %s.%s = %s.id",
				strings.Join(subCols, ", "),
				targetType.Table, tAlias,
				subLatClause,
				tAlias, link.JoinColumn, parentAlias,
			)
		}

		if c.relLoadStrategy == "join" {
			latAlias := tAlias + "_lat"
			lat := fmt.Sprintf(
				"LEFT JOIN LATERAL (SELECT COALESCE(json_agg(row_to_json(%s_sub)), '[]') AS %s FROM (%s) %s_sub) %s ON true",
				tAlias, f.Name, inner, tAlias, latAlias,
			)
			col := fmt.Sprintf("COALESCE(%s.%s, '[]') AS %s", latAlias, f.Name, f.Name)
			return col, lat, nil
		}

		col := fmt.Sprintf(
			"(SELECT COALESCE(json_agg(row_to_json(%s_sub)), '[]') FROM (%s) %s_sub) AS %s",
			tAlias, inner, tAlias, f.Name,
		)
		return col, "", nil
	}

	// Single link → correlated scalar subquery or LEFT JOIN LATERAL.
	joinCond := fmt.Sprintf("%s.id = %s.%s", tAlias, parentAlias, link.JoinColumn)
	inner := fmt.Sprintf(
		"SELECT %s FROM \"%s\" %s%s WHERE %s LIMIT 1",
		strings.Join(subCols, ", "),
		targetType.Table, tAlias,
		subLatClause,
		joinCond,
	)

	if c.relLoadStrategy == "join" {
		latAlias := tAlias + "_lat"
		lat := fmt.Sprintf(
			"LEFT JOIN LATERAL (SELECT row_to_json(%s_sub) AS %s FROM (%s) %s_sub) %s ON true",
			tAlias, f.Name, inner, tAlias, latAlias,
		)
		col := fmt.Sprintf("%s.%s AS %s", latAlias, f.Name, f.Name)
		return col, lat, nil
	}

	col := fmt.Sprintf(
		"(SELECT row_to_json(%s_sub) FROM (%s) %s_sub) AS %s",
		tAlias,
		inner,
		tAlias,
		f.Name,
	)
	return col, "", nil
}

// compileComputedShapeField compiles a shape field with an inline := expression.
func (c *compiler) compileComputedShapeField(f *aql.ShapeField, parentType *asl.ResolvedType, parentAlias string) (string, string, error) {
	expr := f.Computed

	// Pure sub-select: name := (select TypeName { shape } filter ...)
	// A projected subquery — (select ...).field — is a scalar, not a row, so it
	// falls through to scalar compilation below.
	if p := expr.SoloPrimary(); p != nil && p.SubQuery != nil && p.SubQueryField == "" {
		sq := p.SubQuery
		multi := p.SubQueryMulti
		sqRT, err := c.resolveType(sq.TypeName)
		if err != nil {
			return "", "", err
		}
		sqAlias := c.newAlias(sq.TypeName)

		// Build inner SELECT columns.
		var innerCols []string
		var innerLaterals []string
		if sq.Shape != nil {
			innerCols, innerLaterals, err = c.compileShapeCols(sq.Shape, sqRT, sqAlias)
			if err != nil {
				return "", "", err
			}
		} else {
			propNames := make([]string, 0, len(sqRT.Properties))
			for n := range sqRT.Properties {
				propNames = append(propNames, n)
			}
			for _, n := range propNames {
				p := sqRT.Properties[n]
				innerCols = append(innerCols, fmt.Sprintf("%s.%s AS %s", sqAlias, p.Column, p.Name))
			}
		}

		// Build WHERE from filter.
		var where string
		if sq.Filter != nil {
			where, err = c.compileExpr(sq.Filter.Expr, sqAlias, sqRT)
			if err != nil {
				return "", "", err
			}
		}

		innerSQL := fmt.Sprintf(`SELECT %s FROM "%s" %s`, strings.Join(innerCols, ", "), sqRT.Table, sqAlias)
		if len(innerLaterals) > 0 {
			innerSQL += " " + strings.Join(innerLaterals, " ")
		}
		if where != "" {
			innerSQL += " WHERE " + where
		}

		sub := sqAlias + "_" + f.Name + "_sub"
		if multi {
			if c.relLoadStrategy == "join" {
				latAlias := sqAlias + "_" + f.Name + "_lat"
				lat := fmt.Sprintf(`LEFT JOIN LATERAL (SELECT COALESCE(json_agg(row_to_json(%s)), '[]') AS %s FROM (%s) %s) %s ON true`, sub, f.Name, innerSQL, sub, latAlias)
				col := fmt.Sprintf(`COALESCE(%s.%s, '[]') AS %s`, latAlias, f.Name, f.Name)
				return col, lat, nil
			}
			// multi select → JSON array (empty array, not null, when nothing matches).
			col := fmt.Sprintf(`(SELECT COALESCE(json_agg(row_to_json(%s)), '[]') FROM (%s) %s) AS %s`, sub, innerSQL, sub, f.Name)
			return col, "", nil
		}

		if c.relLoadStrategy == "join" {
			latAlias := sqAlias + "_" + f.Name + "_lat"
			lat := fmt.Sprintf(`LEFT JOIN LATERAL (SELECT row_to_json(%s) AS %s FROM (%s LIMIT 1) %s) %s ON true`, sub, f.Name, innerSQL, sub, latAlias)
			col := fmt.Sprintf(`%s.%s AS %s`, latAlias, f.Name, f.Name)
			return col, lat, nil
		}
		// select → single JSON object (null when nothing matches).
		col := fmt.Sprintf(`(SELECT row_to_json(%s) FROM (%s LIMIT 1) %s) AS %s`, sub, innerSQL, sub, f.Name)
		return col, "", nil
	}

	// Scalar computed expression: name := some_expr
	exprSQL, err := c.compileExpr(expr, parentAlias, parentType)
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("(%s) AS %s", exprSQL, f.Name), "", nil
}

// ─────────────────────────────────────────────────────────────
// INSERT
// ─────────────────────────────────────────────────────────────

func (c *compiler) compileInsert(stmt *aql.InsertStmt) (string, error) {
	return c.compileInsertBody(stmt.TypeName, stmt.Assignments, stmt.Conflict, true)
}

func (c *compiler) compileInsertBody(typeName string, assignments []*aql.Assignment, conflict *aql.OnConflict, topLevel bool) (string, error) {
	rt, err := c.resolveType(typeName)
	if err != nil {
		return "", err
	}

	type multiLinkInsert struct {
		link    *asl.ResolvedLink
		isDelta bool
		isFull  bool
		deltas  []*aql.LinkDeltaItem
		fullVal *aql.Expr
	}

	var cols, vals []string
	var ctes []string
	var multiInserts []multiLinkInsert

	for _, a := range assignments {
		if a.LinkDelta != nil {
			link, ok := rt.Links[a.Field]
			if !ok || !link.IsMulti {
				return "", fmt.Errorf("cannot use delta assignment '{ \"+\": ..., \"-\": ... }' on non-multi field %q", a.Field)
			}
			if !topLevel {
				return "", fmt.Errorf("cannot assign multi-link %q in nested insert expression", a.Field)
			}
			for _, item := range a.LinkDelta.Items {
				op := item.NormalizedOp()
				if op != "+" && op != "-" {
					return "", fmt.Errorf("invalid delta operation %q on multi-link %q (expected \"+\" or \"-\")", item.Op, a.Field)
				}
			}
			multiInserts = append(multiInserts, multiLinkInsert{
				link:    link,
				isDelta: true,
				deltas:  a.LinkDelta.Items,
			})
			continue
		}

		// Check if this is a link assignment.
		if link, ok := rt.Links[a.Field]; ok {
			if link.IsMulti {
				if !topLevel {
					return "", fmt.Errorf("cannot assign multi-link %q in nested insert expression", a.Field)
				}
				multiInserts = append(multiInserts, multiLinkInsert{
					link:    link,
					isFull:  true,
					fullVal: a.Value,
				})
				continue
			}
			col, val, cteFrag, err := c.compileLinkAssignment(a, link, rt)
			if err != nil {
				return "", err
			}
			if cteFrag != "" {
				ctes = append(ctes, cteFrag)
			}
			cols = append(cols, col)
			vals = append(vals, val)
			continue
		}
		// Scalar property.
		prop, ok := rt.Properties[a.Field]
		if !ok {
			return "", fmt.Errorf("type %q has no field %q", typeName, a.Field)
		}
		val, err := c.compileExpr(a.Value, "", rt)
		if err != nil {
			return "", err
		}
		inferAssignmentParamType(c.params, a.Value, sqlToAQLType(prop.SQLType), prop.EnumType)
		cols = append(cols, fmt.Sprintf("%q", prop.Column))
		vals = append(vals, val)
	}

	if len(multiInserts) == 0 {
		var sb strings.Builder
		if topLevel {
			// With-block bindings share the statement's single WITH clause with any
			// sub-insert CTEs. Only the top-level insert carries them: a nested
			// (insert ...) operand compiles inside this statement and sees the same
			// bindings already in scope.
			sb.WriteString(c.withPrefix(ctes...))
		} else if len(ctes) > 0 {
			sb.WriteString("WITH ")
			sb.WriteString(strings.Join(ctes, ", "))
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "INSERT INTO \"%s\" (%s)\nVALUES (%s)",
			rt.Table,
			strings.Join(cols, ", "),
			strings.Join(vals, ", "),
		)
		if conflict != nil {
			onConflict, err := c.compileOnConflict(rt, conflict)
			if err != nil {
				return "", err
			}
			sb.WriteString(onConflict)
		}
		if topLevel {
			fmt.Fprintf(&sb, "\nRETURNING %s;", returningColumns(rt))
			return sb.String(), nil
		}
		sb.WriteString(" RETURNING id")
		return sb.String(), nil
	}

	// Top-level insert with multi-link operations
	var insertSQL strings.Builder
	fmt.Fprintf(&insertSQL, "INSERT INTO \"%s\" (%s)\nVALUES (%s)",
		rt.Table,
		strings.Join(cols, ", "),
		strings.Join(vals, ", "),
	)
	if conflict != nil {
		onConflict, err := c.compileOnConflict(rt, conflict)
		if err != nil {
			return "", err
		}
		insertSQL.WriteString(onConflict)
	}
	insertSQL.WriteString("\nRETURNING *")

	targetCTE := fmt.Sprintf("_target AS (\n  %s\n)", strings.ReplaceAll(insertSQL.String(), "\n", "\n  "))
	ctes = append(ctes, targetCTE)

	for _, op := range multiInserts {
		sourceCol := snakeCase(rt.Name)
		targetCol := snakeCase(op.link.TargetType)
		junctionTable := op.link.JunctionTable

		if op.isFull {
			subSQL, err := c.compileMultiLinkTarget(op.fullVal, op.link, rt)
			if err != nil {
				return "", err
			}
			insCTE := fmt.Sprintf("_ins_%s AS (\n  INSERT INTO \"%s\" (\"%s\", \"%s\")\n  SELECT _target.id, _sub.id\n  FROM _target\n  CROSS JOIN (%s) AS _sub(id)\n  ON CONFLICT DO NOTHING\n)",
				op.link.Name, junctionTable, sourceCol, targetCol, subSQL)
			ctes = append(ctes, insCTE)
			continue
		}

		if op.isDelta {
			for _, item := range op.deltas {
				if item.NormalizedOp() == "+" {
					subSQL, err := c.compileMultiLinkTarget(item.Value, op.link, rt)
					if err != nil {
						return "", err
					}
					insCTE := fmt.Sprintf("_ins_%s AS (\n  INSERT INTO \"%s\" (\"%s\", \"%s\")\n  SELECT _target.id, _sub.id\n  FROM _target\n  CROSS JOIN (%s) AS _sub(id)\n  ON CONFLICT DO NOTHING\n)",
						op.link.Name, junctionTable, sourceCol, targetCol, subSQL)
					ctes = append(ctes, insCTE)
					break
				}
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(c.withPrefix(ctes...))
	fmt.Fprintf(&sb, "SELECT %s FROM _target;", returningColumns(rt))
	return sb.String(), nil
}

// compileOnConflict lowers an `unless conflict` clause to a Postgres
// `ON CONFLICT [(cols)] DO NOTHING | DO UPDATE SET ...` fragment (with a leading
// newline). The conflict target, when present, must be backed by an exclusive or
// primary-key constraint; an `else` arm (DO UPDATE) requires a target.
func (c *compiler) compileOnConflict(rt *asl.ResolvedType, oc *aql.OnConflict) (string, error) {
	var cols []string
	if oc.Target != nil {
		for _, f := range oc.Target.Fields {
			col, err := conflictColumn(rt, f)
			if err != nil {
				return "", err
			}
			cols = append(cols, col)
		}
		if !hasUniqueConstraint(rt, oc.Target.Fields, cols) {
			return "", fmt.Errorf(
				"conflict target (%s) on type %q is not backed by an exclusive/primary-key constraint",
				strings.Join(dottedFields(oc.Target.Fields), ", "), rt.Name)
		}
	}

	if oc.Else != nil && len(cols) == 0 {
		return "", fmt.Errorf("`unless conflict ... else` requires an `on` target")
	}

	var sb strings.Builder
	sb.WriteString("\nON CONFLICT")
	if len(cols) > 0 {
		quoted := make([]string, len(cols))
		for i, col := range cols {
			quoted[i] = fmt.Sprintf("%q", col)
		}
		fmt.Fprintf(&sb, " (%s)", strings.Join(quoted, ", "))
	}

	if oc.Else == nil {
		sb.WriteString(" DO NOTHING")
		return sb.String(), nil
	}

	if oc.Else.TypeName != rt.Name {
		return "", fmt.Errorf(
			"else update type %q must match the insert type %q", oc.Else.TypeName, rt.Name)
	}
	sets, err := c.compileConflictSets(rt, oc.Else.Assignments)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&sb, " DO UPDATE SET %s", strings.Join(sets, ", "))
	return sb.String(), nil
}

// compileConflictSets builds the `DO UPDATE SET` assignments, mirroring the SET
// building in compileUpdate but without a table alias (Postgres resolves the
// conflicting row automatically).
func (c *compiler) compileConflictSets(rt *asl.ResolvedType, assignments []*aql.Assignment) ([]string, error) {
	var sets []string
	for _, a := range assignments {
		if prop, ok := rt.Properties[a.Field]; ok {
			val, err := c.compileExpr(a.Value, "", rt)
			if err != nil {
				return nil, err
			}
			inferAssignmentParamType(c.params, a.Value, sqlToAQLType(prop.SQLType), prop.EnumType)
			sets = append(sets, fmt.Sprintf("%q = %s", prop.Column, val))
			continue
		}
		if link, ok := rt.Links[a.Field]; ok {
			if link.IsMulti {
				return nil, fmt.Errorf("cannot assign multi-link %q in conflict update", a.Field)
			}
			val, err := c.compileExpr(a.Value, "", rt)
			if err != nil {
				return nil, err
			}
			inferAssignmentParamType(c.params, a.Value, "uuid", "")
			sets = append(sets, fmt.Sprintf("%q = %s", link.JoinColumn, val))
			continue
		}
		return nil, fmt.Errorf("type %q has no property or link %q", rt.Name, a.Field)
	}
	if len(sets) == 0 {
		return nil, fmt.Errorf("else update must set at least one field")
	}
	return sets, nil
}

// conflictColumn resolves a conflict-target field to its column (scalar property
// column or single-link FK column).
func conflictColumn(rt *asl.ResolvedType, field string) (string, error) {
	if prop, ok := rt.Properties[field]; ok {
		return prop.Column, nil
	}
	if link, ok := rt.Links[field]; ok {
		return link.JoinColumn, nil
	}
	return "", fmt.Errorf("type %q has no field %q in conflict target", rt.Name, field)
}

// hasUniqueConstraint reports whether the conflict target columns are backed by
// an exclusive or primary-key constraint: a field/link-level `exclusive`, or a
// type-level exclusive/pk constraint on the same set of columns.
func hasUniqueConstraint(rt *asl.ResolvedType, fields, cols []string) bool {
	if len(fields) == 1 {
		f := fields[0]
		if prop, ok := rt.Properties[f]; ok {
			for _, con := range prop.Constraints {
				if con.Name == "exclusive" {
					return true
				}
			}
		}
		if link, ok := rt.Links[f]; ok {
			for _, con := range link.Constraints {
				if con.Name == "exclusive" {
					return true
				}
			}
		}
	}
	for _, tc := range rt.Constraints {
		if (tc.Expression == "exclusive" || tc.Expression == "pk") && sameColumnSet(tc.Columns, cols) {
			return true
		}
	}
	return false
}

// sameColumnSet reports whether two column lists contain the same set of names,
// ignoring order (ON CONFLICT target column order is not significant).
func sameColumnSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		if seen[s] == 0 {
			return false
		}
		seen[s]--
	}
	return true
}

// dottedFields renders conflict-target field names as `.name` for error messages.
func dottedFields(fields []string) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = "." + f
	}
	return out
}

// compileLinkAssignment compiles a link assignment. Returns (column, value, cteFrag, error).
// cteFrag is non-empty when a sub-insert CTE was generated.
func (c *compiler) compileLinkAssignment(a *aql.Assignment, link *asl.ResolvedLink, parentType *asl.ResolvedType) (string, string, string, error) {
	if a.LinkDelta != nil || a.Value == nil {
		return "", "", "", fmt.Errorf("cannot use delta assignment on single link %q", a.Field)
	}
	col := fmt.Sprintf("%q", link.JoinColumn)
	operand := a.Value.SoloPrimary()

	// (insert TypeName { ... }) → CTE
	if operand != nil && operand.SubInsert != nil {
		sub := operand.SubInsert
		innerSQL, err := c.compileInsertBody(sub.TypeName, sub.Assignments, nil, false)
		if err != nil {
			return "", "", "", fmt.Errorf("link %q sub-insert: %w", a.Field, err)
		}
		cteAlias := "_ins_" + link.JoinColumn
		cteFrag := fmt.Sprintf("%s AS (%s)", cteAlias, innerSQL)
		val := fmt.Sprintf("(SELECT id FROM %s)", cteAlias)
		return col, val, cteFrag, nil
	}

	// Solo (select TypeName filter ...) without projection or cast → scalar
	// subquery joining on the link's JoinField (which may not be id, so this
	// path can't be folded into the general compileExpr fallback below).
	if operand != nil && operand.SubQuery != nil && operand.SubQueryField == "" && operand.Cast == "" {
		sub := operand.SubQuery

		targetType, err := c.resolveType(link.TargetType)
		if err != nil {
			return "", "", "", err
		}
		alias := c.newAlias(link.TargetType)

		var whereClause string
		if sub.Filter != nil {
			where, err := c.compileValueFilter(sub.Filter.Expr, alias, targetType)
			if err != nil {
				return "", "", "", err
			}
			whereClause = " WHERE " + where
		}

		joinField := link.JoinField
		if joinField == "" {
			joinField = "id"
		}

		subSQL := fmt.Sprintf(
			"(SELECT %s.%s FROM \"%s\" %s%s LIMIT 1)",
			alias, joinField, targetType.Table, alias, whereClause,
		)

		return col, subSQL, "", nil
	}

	// General scalar expression resolving to the target's id — mirrors update's
	// link assignment: a `??` chain of subqueries, a subquery projection
	// ((select X ...).organization), or a bare $param.
	val, err := c.compileExpr(a.Value, "", parentType)
	if err != nil {
		return "", "", "", fmt.Errorf("link %q assignment: %w", a.Field, err)
	}
	inferAssignmentParamType(c.params, a.Value, "uuid", "")
	return col, val, "", nil
}

// compileMultiLinkTarget compiles the target query/expression for a multi-link addition, removal, or replacement.
func (c *compiler) compileMultiLinkTarget(expr *aql.Expr, link *asl.ResolvedLink, parentType *asl.ResolvedType) (string, error) {
	if expr == nil {
		return "", fmt.Errorf("multi-link expression cannot be empty")
	}

	targetType, err := c.resolveType(link.TargetType)
	if err != nil {
		return "", err
	}

	operand := expr.SoloPrimary()

	// 1. With-binding reference: `members := { "+": new_users }`
	if operand != nil && operand.Ident != nil {
		if b, ok := c.lookupWith(*operand.Ident); ok {
			return fmt.Sprintf("SELECT id FROM \"_with_%s\"", b.name), nil
		}
	}

	// 2. Solo SubQuery: `(select User filter ...)` or `(multi select User filter ...)`
	if operand != nil && operand.SubQuery != nil && operand.SubQueryField == "" && operand.Cast == "" {
		sub := operand.SubQuery
		alias := c.newAlias(link.TargetType)

		var whereClause string
		if sub.Filter != nil {
			where, err := c.compileValueFilter(sub.Filter.Expr, alias, targetType)
			if err != nil {
				return "", err
			}
			whereClause = " WHERE " + where
		}

		joinField := link.JoinField
		if joinField == "" {
			joinField = "id"
		}

		subSQL := fmt.Sprintf(
			"SELECT %s.%s AS id FROM \"%s\" %s%s",
			alias, joinField, targetType.Table, alias, whereClause,
		)
		return subSQL, nil
	}

	// 3. Projected subquery, bare param, or general expression
	val, err := c.compileExpr(expr, "", parentType)
	if err != nil {
		return "", err
	}
	inferAssignmentParamType(c.params, expr, "uuid", "")

	trimmed := strings.TrimSpace(val)
	if strings.HasPrefix(trimmed, "(SELECT") && strings.HasSuffix(trimmed, ")") {
		inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, "("), ")")
		return inner, nil
	}
	return fmt.Sprintf("SELECT %s AS id", val), nil
}

// compileSubQuery compiles a (select ...) subquery used as an expression. By
// default it projects the row's id; a non-empty projectField selects that
// property (or link FK) instead — e.g. (select Org filter .id = $id).slug. A
// trailing `<Type>` cast, if any, is applied by the caller to the whole
// subquery.
//
// multi reports whether this is a `multi select`, which yields a set rather than
// a single row — the shape used on the right of `in` (`x in (multi select Y
// filter …)`). A set subquery must NOT be capped at LIMIT 1: doing so turns the
// membership into "member of one arbitrary row". A plain (single) select stays
// scalar and keeps the implicit LIMIT 1.
func (c *compiler) compileSubQuery(body *aql.SelectBody, projectField string, multi bool) (string, error) {
	if body.AggFunc != nil {
		if projectField != "" {
			return "", fmt.Errorf("cannot project field %q from an aggregate subquery", projectField)
		}
		sql, err := c.compileAggSQL(body.AggFunc)
		if err != nil {
			return "", err
		}
		return "(" + sql + ")", nil
	}

	rt, err := c.resolveType(body.TypeName)
	if err != nil {
		return "", err
	}
	alias := c.newAlias(body.TypeName)

	if body.Shape != nil && shapeIsAggregation(body.Shape) {
		if projectField != "" {
			return "", fmt.Errorf("cannot project field %q from an aggregate subquery", projectField)
		}
		cols := make([]string, 0, len(body.Shape.Fields))
		for _, f := range body.Shape.Fields {
			col, err := c.compileAggField(f, rt, alias)
			if err != nil {
				return "", err
			}
			cols = append(cols, col)
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "SELECT %s FROM \"%s\" %s", strings.Join(cols, ", "), rt.Table, alias)
		if body.Filter != nil {
			where, err := c.compileValueFilter(body.Filter.Expr, alias, rt)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&sb, " WHERE %s", where)
		}
		return "(" + sb.String() + ")", nil
	}

	column := "id"
	if projectField != "" {
		col, err := subQueryColumn(rt, projectField)
		if err != nil {
			return "", err
		}
		column = col
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "SELECT %s.%s FROM \"%s\" %s", alias, column, rt.Table, alias)

	if body.Filter != nil {
		where, err := c.compileValueFilter(body.Filter.Expr, alias, rt)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, " WHERE %s", where)
	}

	if multi {
		// A `multi select` subquery is a set (used on the right of `in`); honour an
		// explicit limit/offset but never impose the scalar LIMIT 1.
		if body.Limit != nil {
			limit, err := c.compileExpr(body.Limit, alias, rt)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&sb, " LIMIT %s", limit)
		}
		if body.Offset != nil {
			offset, err := c.compileExpr(body.Offset, alias, rt)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&sb, " OFFSET %s", offset)
		}
	} else {
		sb.WriteString(" LIMIT 1")
	}
	return "(" + sb.String() + ")", nil
}

// subQueryColumn resolves a subquery projection field to a column name: a scalar
// property's column, or a link's FK join column.
func subQueryColumn(rt *asl.ResolvedType, field string) (string, error) {
	if prop, ok := rt.Properties[field]; ok {
		return prop.Column, nil
	}
	if link, ok := rt.Links[field]; ok {
		return link.JoinColumn, nil
	}
	return "", fmt.Errorf("type %q has no field %q to project from subquery", rt.Name, field)
}

// ─────────────────────────────────────────────────────────────
// UPDATE
// ─────────────────────────────────────────────────────────────

func (c *compiler) compileUpdate(stmt *aql.UpdateStmt) (string, error) {
	rt, err := c.resolveType(stmt.TypeName)
	if err != nil {
		return "", err
	}
	alias := c.newAlias(stmt.TypeName)

	type multiLinkUpdate struct {
		link    *asl.ResolvedLink
		isDelta bool
		isFull  bool
		deltas  []*aql.LinkDeltaItem
		fullVal *aql.Expr
	}

	var sets []string
	var multiUpdates []multiLinkUpdate

	for _, a := range stmt.Assignments {
		if a.LinkDelta != nil {
			link, ok := rt.Links[a.Field]
			if !ok || !link.IsMulti {
				return "", fmt.Errorf("cannot use delta assignment '{ \"+\": ..., \"-\": ... }' on non-multi field %q", a.Field)
			}
			for _, item := range a.LinkDelta.Items {
				op := item.NormalizedOp()
				if op != "+" && op != "-" {
					return "", fmt.Errorf("invalid delta operation %q on multi-link %q (expected \"+\" or \"-\")", item.Op, a.Field)
				}
			}
			multiUpdates = append(multiUpdates, multiLinkUpdate{
				link:    link,
				isDelta: true,
				deltas:  a.LinkDelta.Items,
			})
			continue
		}

		if prop, ok := rt.Properties[a.Field]; ok {
			val, err := c.compileExpr(a.Value, alias, rt)
			if err != nil {
				return "", err
			}
			inferAssignmentParamType(c.params, a.Value, sqlToAQLType(prop.SQLType), prop.EnumType)
			sets = append(sets, fmt.Sprintf("%s = %s", prop.Column, val))
			continue
		}

		if link, ok := rt.Links[a.Field]; ok {
			if link.IsMulti {
				multiUpdates = append(multiUpdates, multiLinkUpdate{
					link:    link,
					isFull:  true,
					fullVal: a.Value,
				})
				continue
			}
			val, err := c.compileExpr(a.Value, alias, rt)
			if err != nil {
				return "", err
			}
			inferAssignmentParamType(c.params, a.Value, "uuid", "")
			sets = append(sets, fmt.Sprintf("%s = %s", link.JoinColumn, val))
			continue
		}

		return "", fmt.Errorf("type %q has no property or link %q", stmt.TypeName, a.Field)
	}

	if len(sets) == 0 && len(multiUpdates) == 0 {
		return "", fmt.Errorf("update must set at least one field")
	}

	// Simple update without multi-link operations
	if len(multiUpdates) == 0 {
		var sb strings.Builder
		if len(c.withCTEs) > 0 {
			sb.WriteString(c.withPrefix())
		}
		fmt.Fprintf(&sb, "UPDATE \"%s\" %s SET\n  %s", rt.Table, alias, strings.Join(sets, ",\n  "))

		if stmt.Filter != nil {
			where, err := c.compileExpr(stmt.Filter.Expr, alias, rt)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&sb, "\nWHERE %s", where)
		}
		fmt.Fprintf(&sb, "\nRETURNING %s;", returningColumns(rt))
		return sb.String(), nil
	}

	// Multi-link update CTE pipeline
	var ctes []string

	var whereClause string
	if stmt.Filter != nil {
		where, err := c.compileExpr(stmt.Filter.Expr, alias, rt)
		if err != nil {
			return "", err
		}
		whereClause = fmt.Sprintf("\n  WHERE %s", where)
	}

	var targetCTE string
	if len(sets) > 0 {
		targetCTE = fmt.Sprintf("_target AS (\n  UPDATE \"%s\" %s SET\n    %s%s\n  RETURNING *\n)",
			rt.Table, alias, strings.Join(sets, ",\n    "), whereClause)
	} else {
		targetCTE = fmt.Sprintf("_target AS (\n  SELECT %s.* FROM \"%s\" %s%s\n)",
			alias, rt.Table, alias, whereClause)
	}
	ctes = append(ctes, targetCTE)

	for _, op := range multiUpdates {
		sourceCol := snakeCase(rt.Name)
		targetCol := snakeCase(op.link.TargetType)
		junctionTable := op.link.JunctionTable

		if op.isFull {
			delCTE := fmt.Sprintf("_del_%s AS (\n  DELETE FROM \"%s\"\n  WHERE \"%s\" IN (SELECT id FROM _target)\n)",
				op.link.Name, junctionTable, sourceCol)
			ctes = append(ctes, delCTE)

			subSQL, err := c.compileMultiLinkTarget(op.fullVal, op.link, rt)
			if err != nil {
				return "", err
			}
			insCTE := fmt.Sprintf("_ins_%s AS (\n  INSERT INTO \"%s\" (\"%s\", \"%s\")\n  SELECT _target.id, _sub.id\n  FROM _target\n  CROSS JOIN (%s) AS _sub(id)\n  ON CONFLICT DO NOTHING\n)",
				op.link.Name, junctionTable, sourceCol, targetCol, subSQL)
			ctes = append(ctes, insCTE)
			continue
		}

		if op.isDelta {
			for _, item := range op.deltas {
				if item.NormalizedOp() == "-" {
					subSQL, err := c.compileMultiLinkTarget(item.Value, op.link, rt)
					if err != nil {
						return "", err
					}
					delCTE := fmt.Sprintf("_del_%s AS (\n  DELETE FROM \"%s\"\n  WHERE \"%s\" IN (SELECT id FROM _target)\n    AND \"%s\" IN (%s)\n)",
						op.link.Name, junctionTable, sourceCol, targetCol, subSQL)
					ctes = append(ctes, delCTE)
					break
				}
			}
			for _, item := range op.deltas {
				if item.NormalizedOp() == "+" {
					subSQL, err := c.compileMultiLinkTarget(item.Value, op.link, rt)
					if err != nil {
						return "", err
					}
					insCTE := fmt.Sprintf("_ins_%s AS (\n  INSERT INTO \"%s\" (\"%s\", \"%s\")\n  SELECT _target.id, _sub.id\n  FROM _target\n  CROSS JOIN (%s) AS _sub(id)\n  ON CONFLICT DO NOTHING\n)",
						op.link.Name, junctionTable, sourceCol, targetCol, subSQL)
					ctes = append(ctes, insCTE)
					break
				}
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(c.withPrefix(ctes...))
	fmt.Fprintf(&sb, "SELECT %s FROM _target;", returningColumns(rt))
	return sb.String(), nil
}

// ─────────────────────────────────────────────────────────────
// DELETE
// ─────────────────────────────────────────────────────────────

func (c *compiler) compileDelete(stmt *aql.DeleteStmt) (string, error) {
	rt, err := c.resolveType(stmt.TypeName)
	if err != nil {
		return "", err
	}
	alias := c.newAlias(stmt.TypeName)

	var sb strings.Builder
	fmt.Fprintf(&sb, "DELETE FROM \"%s\" %s", rt.Table, alias)

	if stmt.Filter != nil {
		where, err := c.compileExpr(stmt.Filter.Expr, alias, rt)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "\nWHERE %s", where)
	}
	sb.WriteString(";")
	return sb.String(), nil
}

// ─────────────────────────────────────────────────────────────
// EXPRESSION COMPILATION
// ─────────────────────────────────────────────────────────────

// compileExpr compiles the or-level of a boolean expression. Arms are joined
// with OR; an arm holding more than one comparison is parenthesized so the
// grouping is explicit in the emitted SQL rather than relying on precedence.
func (c *compiler) compileExpr(expr *aql.Expr, alias string, rt *asl.ResolvedType) (string, error) {
	if expr == nil {
		return "", nil
	}

	// orContext is true when this expression joins more than one arm with OR, so
	// an omitted optional param in a lone-comparison arm takes the OR identity
	// (FALSE) rather than the AND identity (see compileCmp). A value filter (the
	// WHERE of a scalar subquery used as a value) forces the same identity: an
	// omitted lookup param must yield no row, not an arbitrary one.
	orContext := len(expr.Rest) > 0 || c.valueFilter

	arms := make([]string, 0, len(expr.Rest)+1)
	for _, a := range append([]*aql.AndExpr{expr.Left}, expr.Rest...) {
		sql, err := c.compileAndExpr(a, alias, rt, orContext)
		if err != nil {
			return "", err
		}
		if len(expr.Rest) > 0 && a != nil && len(a.Rest) > 0 {
			sql = "(" + sql + ")"
		}
		arms = append(arms, sql)
	}
	return strings.Join(arms, " OR "), nil
}

// compileValueFilter compiles the WHERE of a scalar subquery used as a value —
// a link assignment or a `(select ...)` operand. It sets valueFilter for the
// duration so a lone omitted optional param drops the row (subquery → NULL,
// composing with `??`) instead of matching all rows: `(select Org filter .id =
// $org?) ?? ...` must fall through to the fallback when $org is omitted, not
// return an arbitrary organization. AND-combined comparisons keep the match-all
// identity — there the remaining conditions still constrain the lookup.
func (c *compiler) compileValueFilter(expr *aql.Expr, alias string, rt *asl.ResolvedType) (string, error) {
	prev := c.valueFilter
	c.valueFilter = true
	defer func() { c.valueFilter = prev }()
	return c.compileExpr(expr, alias, rt)
}

// compileAndExpr compiles an AND-group. orContext reports whether the enclosing
// expression is a disjunction; a comparison that is AND-combined with siblings
// is always in an AND context (its omitted-optional identity is TRUE), while a
// lone comparison inherits the enclosing connective's identity.
func (c *compiler) compileAndExpr(and *aql.AndExpr, alias string, rt *asl.ResolvedType, orContext bool) (string, error) {
	if and == nil {
		return "", nil
	}

	andContext := len(and.Rest) > 0
	parts := make([]string, 0, len(and.Rest)+1)
	for _, cmp := range append([]*aql.Cmp{and.Left}, and.Rest...) {
		sql, err := c.compileCmp(cmp, alias, rt, orContext && !andContext)
		if err != nil {
			return "", err
		}
		parts = append(parts, sql)
	}
	return strings.Join(parts, " AND "), nil
}

// compileCmp compiles a single comparison. Param-type inference and the
// optional-param null guard live here, per comparison: a `$name?` guard must
// only relax its own comparison, never a whole conjunction. orContext selects
// the guard's identity element: in a disjunction an omitted optional param
// contributes FALSE (it must not satisfy the whole OR), elsewhere TRUE (an
// omitted filter matches all rows).
func (c *compiler) compileCmp(cmp *aql.Cmp, alias string, rt *asl.ResolvedType, orContext bool) (string, error) {
	if cmp == nil {
		return "", nil
	}

	left, err := c.compilePrimary(cmp.Left, alias, rt)
	if err != nil {
		return "", err
	}

	// Postfix null-test: `.x is null` / `.x is not null`.
	if cmp.Is {
		cast := ""
		if cmp.Left != nil && cmp.Left.Param != nil && !strings.Contains(left, "::") {
			cast = c.paramCastSuffix(cmp.Left, nil, rt)
		}
		if cmp.IsNot {
			return left + cast + " IS NOT NULL", nil
		}
		return left + cast + " IS NULL", nil
	}

	if cmp.Op == "" {
		return left, nil
	}

	// Membership: `<value> in .<path>` where the path ends in a multi-link is a
	// set test, lowered to `<value> IN (SELECT … FROM junction …)` rather than a
	// scalar comparison. Intercept before compilePrimary tries (and fails) to
	// lower the multi-link path as a scalar. A non-multi RHS falls through to the
	// ordinary infix `in`.
	if cmp.Op == "in" && cmp.Right != nil && cmp.Right.Path != nil {
		if membership, ok, err := c.compileMembership(left, cmp.Right.Path, alias, rt); err != nil {
			return "", err
		} else if ok {
			return membership, nil
		}
	}

	// Membership over a `multi` with-binding: `.sender_id in api_keys.id` or
	// `.sender_id in api_keys.id<str>`. This is the one position where a
	// set-valued binding is meaningful, so it is taken here before compilePrimary
	// rejects it as a non-scalar. A trailing cast (<str>, <uuid>, …) is applied
	// inside the subquery projection so Postgres sees the cast at the column
	// level: `IN (SELECT (_with_api_key.id)::TEXT FROM _with_api_key)`.
	if cmp.Op == "in" && cmp.Right != nil {
		if b, field, ok := c.withOperand(cmp.Right); ok && b.multi {
			set, err := c.withSetRefCast(b, field, cmp.Right.Cast)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s IN %s", left, set), nil
		}
	}

	right, err := c.compilePrimary(cmp.Right, alias, rt)
	if err != nil {
		return "", err
	}

	// Infer param types from the opposite side of a comparison.
	if rt != nil {
		c.inferFilterParamType(cmp.Left, cmp.Right, rt)
		c.inferFilterParamType(cmp.Right, cmp.Left, rt)
	}
	c.inferWithParamType(cmp.Left, cmp.Right)
	c.inferWithParamType(cmp.Right, cmp.Left)

	// Null-coalesce ($x ?? .field) is a function, not an infix operator: emit
	// COALESCE(left, right). A param operand is cast to the SQL type of the
	// opposite operand so its type is determinable when the value is null.
	if cmp.Op == "??" {
		lc := left
		if !strings.Contains(left, "::") {
			lc += c.paramCastSuffix(cmp.Left, cmp.Right, rt)
		}
		rc := right
		if !strings.Contains(right, "::") {
			rc += c.paramCastSuffix(cmp.Right, cmp.Left, rt)
		}
		return fmt.Sprintf("COALESCE(%s, %s)", lc, rc), nil
	}

	result := fmt.Sprintf("%s %s %s", left, cmp.Op, right)

	// Optional params ($name?) make the comparison a no-op when the value is
	// null. The identity that "no-op" collapses to depends on the enclosing
	// connective: in an AND (or a lone top-level filter) an omitted param matches
	// all rows — `($N IS NULL OR result)`; in an OR it must instead drop out of
	// the disjunction — `($N IS NOT NULL AND result)` — otherwise one omitted
	// param makes the whole OR true and silently voids the other arms.
	//
	// The standalone `$N IS [NOT] NULL` occurrence carries no type, so cast it to
	// the SQL type of the column it's compared against — otherwise Postgres can't
	// determine the parameter type when the value is null (42P08). Casting to the
	// column type keeps the cast consistent with the comparison (avoiding e.g. a
	// str-vs-uuid conflict).
	for _, operand := range []*aql.Primary{cmp.Left, cmp.Right} {
		if operand != nil && operand.Param != nil && operand.Param.Optional {
			ph := c.params.add(operand.Param.Name, "")
			other := cmp.Right
			if operand == cmp.Right {
				other = cmp.Left
			}
			cast := c.paramCastSuffix(operand, other, rt)
			if orContext {
				result = fmt.Sprintf("(%s%s IS NOT NULL AND %s)", ph, cast, result)
			} else {
				result = fmt.Sprintf("(%s%s IS NULL OR %s)", ph, cast, result)
			}
		}
	}

	return result, nil
}

func (c *compiler) compilePrimary(p *aql.Primary, alias string, rt *asl.ResolvedType) (string, error) {
	if p == nil {
		return "", fmt.Errorf("nil primary expression")
	}
	sql, err := c.compilePrimaryValue(p, alias, rt)
	if err != nil {
		return "", err
	}
	// A trailing `<Type>` cast wraps whatever the operand compiled to.
	if p.Cast != "" {
		sqlType, err := c.annotSQLType(p.Cast)
		if err != nil {
			return "", err
		}
		sql = fmt.Sprintf("(%s)::%s", sql, sqlType)
	}
	return sql, nil
}

// compilePrimaryValue compiles the operand itself, without the trailing cast.
func (c *compiler) compilePrimaryValue(p *aql.Primary, alias string, rt *asl.ResolvedType) (string, error) {
	switch {
	case p.SubQuery != nil:
		if p.SubQueryMulti && p.SubQueryField != "" {
			return "", fmt.Errorf("cannot project field %q from a `multi select` (it yields a set, not a row)", p.SubQueryField)
		}
		return c.compileSubQuery(p.SubQuery, p.SubQueryField, p.SubQueryMulti)

	case p.SubInsert != nil:
		// (insert TypeName { ... }) used as a scalar — compile as a subquery returning id.
		sql, err := c.compileInsertBody(p.SubInsert.TypeName, p.SubInsert.Assignments, nil, false)
		if err != nil {
			return "", err
		}
		return "(" + sql + ")", nil

	case p.SubExpr != nil:
		inner, err := c.compileExpr(p.SubExpr, alias, rt)
		if err != nil {
			return "", err
		}
		return "(" + inner + ")", nil

	case p.FuncCall != nil:
		return c.compileFuncCall(p.FuncCall, alias, rt)

	case p.Path != nil:
		return c.compilePath(p.Path, alias, rt)

	case p.Param != nil:
		if c.policyMode {
			return "", fmt.Errorf("policy predicates can't use bind parameters ($%s); reference a `global` instead", p.Param.Name)
		}
		// In a function body, $name is a declared plpgsql argument, emitted by name.
		if c.trig != nil {
			if c.trig.params[p.Param.Name] {
				return p.Param.Name, nil
			}
			return "", fmt.Errorf("unknown parameter $%s in trigger/function body", p.Param.Name)
		}
		aqlType, enumType, err := c.resolveParamType(p.Param.Name, p.Param.Type)
		if err != nil {
			return "", err
		}
		ph := c.params.add(p.Param.Name, aqlType)
		if aqlType != "" {
			c.params.setExplicitType(p.Param.Name, aqlType)
		}
		if enumType != "" {
			c.params.setEnumType(p.Param.Name, enumType)
		}
		if p.Param.Optional {
			c.params.markOptional(p.Param.Name)
		}
		if aqlType == "" && c.params.isExplicit(p.Param.Name) {
			aqlType = paramAQLType(c.params, p.Param.Name)
		}
		if aqlType != "" && (p.Param.Type != "" || c.params.isExplicit(p.Param.Name)) {
			if sqlType, ok := asl.BuiltinSQLType(aqlType); ok {
				return fmt.Sprintf("%s::%s", ph, sqlType), nil
			}
		}
		return ph, nil

	case p.Null:
		return "NULL", nil
	case p.True:
		return "true", nil
	case p.False:
		return "false", nil
	case p.Str != nil:
		return *p.Str, nil
	case p.Int != nil:
		return *p.Int, nil
	case p.Float != nil:
		return *p.Float, nil
	case p.GlobalRef != nil:
		return c.compileGlobalRef(*p.GlobalRef)
	case p.QualifiedIdent != nil:
		qi := p.QualifiedIdent
		// A `with` binding shadows a type or enum of the same name, so it is
		// resolved first: business.id reads the bound row's id column.
		if b, ok := c.lookupWith(qi.TypeName); ok {
			return c.withRef(b, qi.Field)
		}
		// Trigger magic: __new__.field / __old__.field / __subject__.field →
		// NEW."col" / OLD."col", validated against the enclosing type.
		if c.trig != nil {
			if row, ok := triggerRowRef(qi.TypeName); ok {
				return c.compileTriggerField(row, qi.Field, qi.Fields)
			}
		}
		// Enum member reference: EnumName.Value → SQL string literal. Enums are
		// stored as TEXT, so `.status = MyEnum.Value` lowers the RHS to 'Value'.
		if enum, ok := c.schema.EnumTypes[qi.TypeName]; ok {
			for _, v := range enum.Values {
				if v == qi.Field {
					return "'" + qi.Field + "'", nil
				}
			}
			return "", fmt.Errorf("enum %q has no value %q", qi.TypeName, qi.Field)
		}
		qrt := c.schema.ObjectTypes[qi.TypeName]
		if qrt == nil {
			return "", fmt.Errorf("unknown type %q in qualified reference", qi.TypeName)
		}
		outerAlias := tableAlias(qi.TypeName)
		if prop, ok := qrt.Properties[qi.Field]; ok {
			return fmt.Sprintf("%s.%s", outerAlias, prop.Column), nil
		}
		if link, ok := qrt.Links[qi.Field]; ok {
			return fmt.Sprintf("%s.%s", outerAlias, link.JoinColumn), nil
		}
		return "", fmt.Errorf("type %q has no field %q", qi.TypeName, qi.Field)

	case p.Ident != nil:
		// A bare binding reference means its row's id, so `business is not null`
		// reads as "the binding matched a row".
		if b, ok := c.lookupWith(*p.Ident); ok {
			return c.withRef(b, "")
		}
		// Trigger magic: bare __new__/__old__/__subject__ (whole row) and event.
		if c.trig != nil {
			switch *p.Ident {
			case "__new__", "__subject__":
				return "NEW", nil
			case "__old__":
				return "OLD", nil
			case "event":
				return "TG_OP", nil
			}
		}
		return *p.Ident, nil
	}

	return "", fmt.Errorf("empty primary expression")
}

// compileGlobalRef lowers `global <name>` to a read of its backing Postgres
// session setting: current_setting('app.<name>', <missing_ok>)::<sqltype>. A
// required global uses missing_ok=false (error when the GUC is unset — fail
// closed); an optional global uses true (NULL when unset).
func (c *compiler) compileGlobalRef(name string) (string, error) {
	for _, g := range c.schema.Globals {
		if g.Name == name {
			missingOK := "true"
			if g.Required {
				missingOK = "false"
			}
			return fmt.Sprintf("current_setting('app.%s', %s)::%s", g.Name, missingOK, g.SQLType), nil
		}
	}
	return "", fmt.Errorf("unknown global %q", name)
}

// triggerRowRef reports whether name is a trigger row alias and which SQL row it
// maps to (NEW / OLD).
func triggerRowRef(name string) (string, bool) {
	switch name {
	case "__new__", "__subject__":
		return "NEW", true
	case "__old__":
		return "OLD", true
	}
	return "", false
}

// compileTriggerField resolves a magic-row field access to NEW/OLD."column",
// validating the field against the enclosing type when one is bound.
func (c *compiler) compileTriggerField(row, field string, subfields []string) (string, error) {
	if c.trig.enclosing == nil {
		// Standalone function: no table bound, pass the field through snake-cased.
		col := fmt.Sprintf(`%s.%q`, row, snakeCase(field))
		for _, sub := range subfields {
			col += fmt.Sprintf("->>%q", sub)
		}
		return col, nil
	}
	rt := c.trig.enclosing
	if prop, ok := rt.Properties[field]; ok {
		col := fmt.Sprintf("%s.%q", row, prop.Column)
		for _, sub := range subfields {
			col += fmt.Sprintf("->>%q", sub)
		}
		return col, nil
	}
	if link, ok := rt.Links[field]; ok {
		if len(subfields) == 0 || (len(subfields) == 1 && subfields[0] == "id") {
			return fmt.Sprintf("%s.%q", row, link.JoinColumn), nil
		}
	}
	return "", fmt.Errorf("type %q has no field %q (in trigger row reference)", rt.Name, field)
}

func (c *compiler) compilePath(path *aql.PathExpr, alias string, rt *asl.ResolvedType) (string, error) {
	if len(path.Steps) == 0 {
		return "", fmt.Errorf("empty path expression")
	}

	// Single step: .fieldName → alias.column_name
	if len(path.Steps) == 1 {
		name := path.Steps[0]

		if prop, ok := rt.Properties[name]; ok {
			if alias != "" {
				return fmt.Sprintf("%s.%s", alias, prop.Column), nil
			}
			return prop.Column, nil
		}

		if link, ok := rt.Links[name]; ok {
			if alias != "" {
				return fmt.Sprintf("%s.%s", alias, link.JoinColumn), nil
			}
			return link.JoinColumn, nil
		}

		if comp, ok := rt.Computed[name]; ok {
			return expandComputedExpr(comp.Expr, alias), nil
		}

		return "", fmt.Errorf("type %q has no field %q", rt.Name, name)
	}

	// Multi-step:
	first := path.Steps[0]

	// 1. JSON / Typed JSON scalar property access: .coord.lat or .data.status
	if prop, ok := rt.Properties[first]; ok {
		var scalar *asl.ResolvedScalar
		if c.schema != nil && prop.AQLType != "" {
			scalar = c.schema.ScalarTypes[prop.AQLType]
		}
		isJSON := prop.SQLType == "JSON" || prop.SQLType == "JSONB" || (scalar != nil && (scalar.SQLType == "JSON" || scalar.SQLType == "JSONB" || len(scalar.Fields) > 0))
		if isJSON {
			remaining := path.Steps[1:]
			colRef := prop.Column
			if alias != "" {
				colRef = fmt.Sprintf("%s.%s", alias, prop.Column)
			}
			if scalar != nil && len(scalar.Fields) > 0 {
				if len(remaining) > 1 {
					return "", fmt.Errorf("cannot traverse nested path in scalar %q (nested traversal is not supported)", scalar.Name)
				}
				field, ok := scalar.Fields[remaining[0]]
				if !ok {
					return "", fmt.Errorf("scalar type %q has no field %q", scalar.Name, remaining[0])
				}
				if field.ExprSQL != "" {
					expanded := strings.ReplaceAll(field.ExprSQL, "__self__", colRef)
					return expanded, nil
				}
				if field.SQLType == "TEXT" {
					return fmt.Sprintf("(%s->>'%s')", colRef, remaining[0]), nil
				}
				return fmt.Sprintf("((%s->>'%s')::%s)", colRef, remaining[0], field.SQLType), nil
			}
			// Untyped JSON property:
			if len(remaining) == 1 {
				return fmt.Sprintf("(%s->>'%s')", colRef, remaining[0]), nil
			}
			var parts []string
			for i, step := range remaining {
				if i == len(remaining)-1 {
					parts = append(parts, fmt.Sprintf("->>'%s'", step))
				} else {
					parts = append(parts, fmt.Sprintf("->'%s'", step))
				}
			}
			return fmt.Sprintf("(%s%s)", colRef, strings.Join(parts, "")), nil
		}
	}

	// 2. Link traversal
	linkName := path.Steps[0]
	link, ok := rt.Links[linkName]
	if !ok {
		return "", fmt.Errorf("type %q has no link %q", rt.Name, linkName)
	}

	// A multi-link can't appear in a scalar path — it yields a set, not a value.
	// Membership tests go through `<value> in .<path>` (see compileMembership).
	if link.IsMulti {
		return "", fmt.Errorf("can't traverse the multi-link %q in a scalar path (.%s); use `<value> in .<path>` for a membership test", linkName, strings.Join(path.Steps, "."))
	}

	targetType, err := c.resolveType(link.TargetType)
	if err != nil {
		return "", err
	}

	tAlias := c.newAlias(link.TargetType)
	remaining := path.Steps[1:]

	joinCond := link.JoinColumn
	if joinCond == "" {
		joinCond = strings.ToLower(linkName) + "_id"
	}

	// Optimization: .link.id → FK column directly, avoiding a correlated subquery
	// and alias conflicts when the outer query already uses the same alias.
	if len(remaining) == 1 && remaining[0] == "id" {
		return outerRef(alias, rt, joinCond), nil
	}

	subPath := &aql.PathExpr{Steps: remaining}
	subExpr, err := c.compilePath(subPath, tAlias, targetType)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"(SELECT %s FROM \"%s\" %s WHERE %s.id = %s LIMIT 1)",
		subExpr, targetType.Table, tAlias, tAlias, outerRef(alias, rt, joinCond),
	), nil
}

// outerRef qualifies a column on the row compilePath is resolving against. With a
// normal query alias it is `alias.col`. In policy mode the base row has no alias
// (RLS predicates run with the target row's columns unqualified in scope), so it
// is referenced by table name — `"table".col` — which also disambiguates a
// correlated subquery from a self-referential link's own columns.
func outerRef(alias string, rt *asl.ResolvedType, col string) string {
	if alias != "" {
		return fmt.Sprintf("%s.%s", alias, col)
	}
	return fmt.Sprintf("%q.%s", rt.Table, col)
}

// compileMembership lowers `<left> in .<path>` when the path ends in a multi-link
// — a membership test over the link's junction (or target FK). It returns
// (sql, true, nil) when the path terminates in a multi-link, and ("", false, nil)
// when it does not, so compileCmp falls back to the ordinary infix `in`. Any
// leading steps must be single links (they locate the row that owns the
// multi-link); the emitted subquery correlates back to that owner row.
func (c *compiler) compileMembership(left string, path *aql.PathExpr, alias string, rt *asl.ResolvedType) (string, bool, error) {
	if path == nil || len(path.Steps) == 0 {
		return "", false, nil
	}

	// Walk the single-link prefix to the type that owns the multi-link.
	ownerType := rt
	for _, step := range path.Steps[:len(path.Steps)-1] {
		l, ok := ownerType.Links[step]
		if !ok || l.IsMulti {
			return "", false, nil // not a clean single-link prefix → not a membership test
		}
		t, err := c.resolveType(l.TargetType)
		if err != nil {
			return "", false, err
		}
		ownerType = t
	}

	last := path.Steps[len(path.Steps)-1]
	link, ok := ownerType.Links[last]
	if !ok || !link.IsMulti {
		return "", false, nil // terminal isn't a multi-link → let the ordinary `in` handle it
	}

	targetType, err := c.resolveType(link.TargetType)
	if err != nil {
		return "", false, err
	}

	// ownerRef: SQL scalar for the id of the row that owns the multi-link. With no
	// prefix that's the base row's id; otherwise reuse compilePath to resolve
	// `.<prefix>.id` (the FK column directly, or a correlated subquery for deeper
	// chains).
	var ownerRef string
	if len(path.Steps) == 1 {
		ownerRef = outerRef(alias, rt, "id")
	} else {
		idSteps := append(append([]string{}, path.Steps[:len(path.Steps)-1]...), "id")
		ownerRef, err = c.compilePath(&aql.PathExpr{Steps: idSteps}, alias, rt)
		if err != nil {
			return "", false, err
		}
	}

	joinField := link.JoinField
	if joinField == "" {
		joinField = "id"
	}
	tAlias := c.newAlias(link.TargetType)

	var inner string
	if link.JunctionTable != "" {
		// Junction table with one FK column per side, named after the referenced
		// table (see generateJunctionTable / compileLinkField): ownerType.Table on
		// the owner side, targetType.Table on the target side.
		jAlias := "jt"
		inner = fmt.Sprintf(
			"SELECT %s.%s FROM %q %s JOIN %q %s ON %s.%s = %s.%s WHERE %s.%s = %s",
			tAlias, joinField,
			link.JunctionTable, jAlias,
			targetType.Table, tAlias,
			tAlias, joinField, jAlias, targetType.Table,
			jAlias, ownerType.Table, ownerRef,
		)
	} else {
		// Direct FK on the target side (rare for multi).
		inner = fmt.Sprintf(
			"SELECT %s.%s FROM %q %s WHERE %s.%s = %s",
			tAlias, joinField,
			targetType.Table, tAlias,
			tAlias, link.JoinColumn, ownerRef,
		)
	}

	return fmt.Sprintf("%s IN (%s)", left, inner), true, nil
}

func (c *compiler) compileFuncCall(fc *aql.FuncCall, alias string, rt *asl.ResolvedType) (string, error) {
	fn := fc.Name
	if aql.AggFuncs[strings.ToLower(fn)] {
		fn = strings.ToUpper(fn)
	}
	if strings.ToLower(fc.Name) == "count" && len(fc.Args) == 0 {
		return "COUNT(*)", nil
	}
	if c.schema != nil {
		if declFn, isLocal := c.schema.Functions[fc.Name]; isLocal {
			if len(fc.Args) != len(declFn.Params) {
				return "", fmt.Errorf("function %q expects %d argument(s), got %d", fc.Name, len(declFn.Params), len(fc.Args))
			}
			for i, a := range fc.Args {
				if p := a.SoloPrimary(); p != nil {
					var argType string
					switch {
					case p.Str != nil:
						argType = "TEXT"
					case p.Int != nil:
						argType = "INTEGER"
					case p.Float != nil:
						argType = "NUMERIC"
					case p.True || p.False:
						argType = "BOOLEAN"
					}
					if argType != "" {
						expected := declFn.Params[i].SQLType
						if !isTypeCompatible(argType, expected) {
							return "", fmt.Errorf("function %q argument %d expects %s, got %s",
								fc.Name, i+1, sqlToAQLType(expected), sqlToAQLType(argType))
						}
					}
				}
			}
		}
	}
	var args []string
	for _, a := range fc.Args {
		s, err := c.compileExpr(a, alias, rt)
		if err != nil {
			return "", err
		}
		if p := a.SoloPrimary(); p != nil && p.Param != nil && p.Cast == "" && !strings.Contains(s, "::") {
			s += c.paramCastSuffix(p, nil, rt)
		}
		args = append(args, s)
	}
	return fmt.Sprintf("%s(%s)", fn, strings.Join(args, ", ")), nil
}

func isTypeCompatible(argType, expectedSQL string) bool {
	expectedSQL = strings.ToUpper(strings.TrimSpace(expectedSQL))
	if argType == "UNKNOWN" || expectedSQL == "" {
		return true
	}
	switch argType {
	case "TEXT":
		return expectedSQL == "TEXT" || expectedSQL == "VARCHAR" || strings.HasPrefix(expectedSQL, "VARCHAR") || expectedSQL == "CHAR"
	case "INTEGER":
		return expectedSQL == "INTEGER" || expectedSQL == "SMALLINT" || expectedSQL == "BIGINT" || expectedSQL == "NUMERIC" || expectedSQL == "REAL" || expectedSQL == "DOUBLE PRECISION" || expectedSQL == "DECIMAL"
	case "NUMERIC":
		return expectedSQL == "NUMERIC" || expectedSQL == "REAL" || expectedSQL == "DOUBLE PRECISION" || expectedSQL == "DECIMAL" || expectedSQL == "FLOAT"
	case "BOOLEAN":
		return expectedSQL == "BOOLEAN" || expectedSQL == "BOOL"
	default:
		return strings.EqualFold(argType, expectedSQL)
	}
}

// ─────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────

func (c *compiler) resolveType(name string) (*asl.ResolvedType, error) {
	rt, ok := c.schema.ObjectTypes[name]
	if !ok {
		return nil, fmt.Errorf("unknown type %q", name)
	}
	return rt, nil
}

// tableAlias returns a short lowercase alias for a type name. It is the base
// (un-numbered) form; use c.newAlias for a fresh table instance in a query so
// two tables sharing a first letter can't collide.
func tableAlias(typeName string) string {
	if len(typeName) == 0 {
		return "t"
	}
	return strings.ToLower(string(typeName[0]))
}

// newAlias returns a unique alias for a new table instance within this compile.
// The first table to claim a given base letter keeps the bare alias (so the
// common single-table and non-colliding cases emit unchanged SQL); each later
// table sharing that letter is numbered ("w", then "w1", "w2", …). Because every
// table occurrence gets a distinct alias, a nested/correlated subquery can no
// longer shadow an outer table that happens to derive the same letter, while
// correlated column references keep binding to the outer scope's alias (threaded
// through compilePath's `alias` parameter).
func (c *compiler) newAlias(typeName string) string {
	base := tableAlias(typeName)
	if c.aliasCounts == nil {
		c.aliasCounts = map[string]int{}
	}
	n := c.aliasCounts[base]
	c.aliasCounts[base]++
	if n == 0 {
		return base
	}
	return fmt.Sprintf("%s%d", base, n)
}

func (c *compiler) compileVars(vars []*aql.VarBlock) error {
	for _, vb := range vars {
		if vb == nil {
			continue
		}
		for _, p := range vb.Params {
			if p == nil {
				continue
			}
			baseType, enumType, err := c.resolveParamType(p.Name, p.Type)
			if err != nil {
				return err
			}
			c.params.add(p.Name, baseType)
			if p.Type != "" {
				c.params.setExplicitType(p.Name, baseType)
			}
			if enumType != "" {
				c.params.setEnumType(p.Name, enumType)
			}
			if p.Optional {
				c.params.markOptional(p.Name)
			}
		}
	}
	return nil
}

// resolveParamType classifies an inline param annotation ($name<type>) against
// the schema. It accepts any declared value type — a builtin scalar, a scalar
// alias, or an enum — and rejects object types (tables), since a param is a
// value, not a row. Returns (aqlBaseType, enumTypeName, error). An empty
// annotation yields ("", "", nil) so type inference can fill it in later.
func (c *compiler) resolveParamType(name, annot string) (string, string, error) {
	if annot == "" {
		return "", "", nil
	}
	if _, ok := asl.BuiltinSQLType(annot); ok {
		return annot, "", nil
	}
	if e, ok := c.schema.EnumTypes[annot]; ok {
		return "str", e.Name, nil
	}
	if s, ok := c.schema.ScalarTypes[annot]; ok {
		return s.Base, "", nil
	}
	if _, ok := c.schema.ObjectTypes[annot]; ok {
		return "", "", fmt.Errorf("$%s: %q is an object type (table), not usable as a parameter type", name, annot)
	}
	return "", "", fmt.Errorf("$%s: unknown parameter type %q", name, annot)
}

// annotSQLType resolves an inline type annotation (<str>, <MyEnum>, <MyAlias>)
// to a SQL type for an explicit cast, using the same classification as
// resolveParamType: builtin scalar, scalar alias, or enum (stored as TEXT).
// Object types are rejected.
func (c *compiler) annotSQLType(annot string) (string, error) {
	if sqlType, ok := asl.BuiltinSQLType(annot); ok {
		return sqlType, nil
	}
	if _, ok := c.schema.EnumTypes[annot]; ok {
		return "TEXT", nil
	}
	if s, ok := c.schema.ScalarTypes[annot]; ok {
		if sqlType, ok := asl.BuiltinSQLType(s.Base); ok {
			return sqlType, nil
		}
	}
	if _, ok := c.schema.ObjectTypes[annot]; ok {
		return "", fmt.Errorf("cannot cast to %q: it is an object type (table)", annot)
	}
	return "", fmt.Errorf("unknown cast type %q", annot)
}

// sqlToAQLType maps a SQL type string back to an AQL type name.
func sqlToAQLType(sqlType string) string {
	switch sqlType {
	case "TEXT":
		return "str"
	case "SMALLINT":
		return "int16"
	case "INTEGER":
		return "int32"
	case "BIGINT":
		return "int64"
	case "REAL":
		return "float32"
	case "DOUBLE PRECISION":
		return "float64"
	case "BOOLEAN":
		return "bool"
	case "UUID":
		return "uuid"
	case "TIMESTAMPTZ":
		return "datetime"
	case "DATE":
		return "date"
	case "TIME":
		return "time"
	case "JSON":
		return "json"
	case "JSONB":
		return "jsonb"
	case "BYTEA":
		return "bytes"
	case "NUMERIC":
		return "decimal"
	default:
		return ""
	}
}

// inferAssignmentParamType sets the param type (and enum type, when the target
// column is enum-backed) when an assignment value is a bare $param.
func inferAssignmentParamType(params *paramCollector, val *aql.Expr, aqlType, enumType string) {
	if p := val.SoloPrimary(); p != nil && p.Param != nil {
		params.setType(p.Param.Name, aqlType)
		if enumType != "" {
			params.setEnumType(p.Param.Name, enumType)
		}
	}
}

// inferFilterParamType sets a param's type when paired with a path on the other side of a binary op.
func (c *compiler) inferFilterParamType(maybePath, maybeParam *aql.Primary, rt *asl.ResolvedType) {
	if maybePath == nil || maybeParam == nil || maybeParam.Param == nil || rt == nil {
		return
	}
	if maybePath.Path != nil {
		if len(maybePath.Path.Steps) == 1 {
			if prop, ok := rt.Properties[maybePath.Path.Steps[0]]; ok {
				c.params.setType(maybeParam.Param.Name, sqlToAQLType(prop.SQLType))
				if prop.EnumType != "" {
					c.params.setEnumType(maybeParam.Param.Name, prop.EnumType)
				}
			}
		} else if len(maybePath.Path.Steps) == 2 {
			if prop, ok := rt.Properties[maybePath.Path.Steps[0]]; ok {
				if c.schema != nil && prop.AQLType != "" {
					if scalar, ok := c.schema.ScalarTypes[prop.AQLType]; ok && scalar.Fields != nil {
						if field, ok := scalar.Fields[maybePath.Path.Steps[1]]; ok {
							c.params.setType(maybeParam.Param.Name, field.AQLType)
						}
					}
				}
			}
		}
	}
}

// filterOperandSQLType returns the SQL type of a single-step path operand — a
// scalar property's SQL type, or UUID for a link's FK column — or a typed JSON field's SQL type.
// Used to cast an optional param's `IS NULL` check so its type is known even when null.
func (c *compiler) filterOperandSQLType(p *aql.Primary, rt *asl.ResolvedType) string {
	if p == nil || rt == nil || p.Path == nil {
		return ""
	}
	if len(p.Path.Steps) == 1 {
		name := p.Path.Steps[0]
		if prop, ok := rt.Properties[name]; ok {
			return prop.SQLType
		}
		if _, ok := rt.Links[name]; ok {
			return "UUID" // FK columns reference the target's uuid id
		}
	} else if len(p.Path.Steps) == 2 {
		if prop, ok := rt.Properties[p.Path.Steps[0]]; ok {
			if c.schema != nil && prop.AQLType != "" {
				if scalar, ok := c.schema.ScalarTypes[prop.AQLType]; ok && scalar.Fields != nil {
					if field, ok := scalar.Fields[p.Path.Steps[1]]; ok {
						return field.SQLType
					}
				}
			}
		}
	}
	return ""
}

// paramCastSuffix returns a "::<sqltype>" cast for operand when it is a param, so
// its type stays determinable when the value is null (COALESCE args and optional
// `IS NULL` checks). The type is taken from the opposite operand's column when
// available, else from the param's own resolved AQL type. Returns "" when operand
// is not a param.
func (c *compiler) paramCastSuffix(operand, other *aql.Primary, rt *asl.ResolvedType) string {
	if operand == nil || operand.Param == nil {
		return ""
	}
	if t := c.filterOperandSQLType(other, rt); t != "" {
		return "::" + t
	}
	if bt, ok := asl.BuiltinSQLType(paramAQLType(c.params, operand.Param.Name)); ok {
		return "::" + bt
	}
	return ""
}

// paramAQLType returns the collected AQL type of a named param (e.g. "str"), or
// "" if unknown.
func paramAQLType(params *paramCollector, name string) string {
	if pos, ok := params.index[name]; ok {
		return params.params[pos-1].AQLType
	}
	return ""
}

// expandComputedExpr replaces `.field` references with `alias.field` in a
// raw computed expression string (stored as joined token parts).
func expandComputedExpr(expr, alias string) string {
	// The expression is stored as token parts joined together, e.g. ".name??.email"
	// Replace leading dots with alias prefix.
	parts := strings.Fields(expr)
	var result []string
	for _, p := range parts {
		if strings.HasPrefix(p, ".") {
			result = append(result, alias+p)
		} else if p == "??" {
			result = append(result, "COALESCE")
		} else {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return expr
	}
	return strings.Join(result, " ")
}
