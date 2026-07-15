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

// Compile compiles a parsed AQL statement against a SchemaIR into SQL.
func Compile(stmt *aql.Statement, schema *asl.SchemaIR) (*CompiledSQL, error) {
	c := &compiler{schema: schema, params: newParamCollector()}
	var sql string
	var err error

	switch {
	case stmt.Select != nil:
		sql, err = c.compileSelect(stmt.Select)
	case stmt.Insert != nil:
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

	return &CompiledSQL{
		SQL:    sql,
		Params: c.params.params,
	}, nil
}

type compiler struct {
	schema *asl.SchemaIR
	params *paramCollector
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

	alias := tableAlias(typeName)
	table := rt.Table

	// Build SELECT columns from shape (or "*" if no shape).
	var cols []string
	var laterals []string

	if body.Shape != nil {
		// Collect explicitly-named fields so a `*` splat can skip them (explicit
		// selections win over the splat expansion).
		explicit := make(map[string]bool)
		for _, f := range body.Shape.Fields {
			if !f.Star {
				explicit[f.Name] = true
			}
		}
		for _, f := range body.Shape.Fields {
			if f.Star {
				// Expand to all scalar props + single-link FK columns not named
				// explicitly elsewhere in the shape.
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
			col, lateral, err := c.compileShapeField(f, rt, alias)
			if err != nil {
				return "", err
			}
			cols = append(cols, col)
			if lateral != "" {
				laterals = append(laterals, lateral)
			}
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
		sb.WriteString(",\n")
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
	rt, err := c.resolveType(agg.TypeName)
	if err != nil {
		return "", err
	}
	alias := tableAlias(agg.TypeName)

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
		return fmt.Sprintf("SELECT COUNT(*) FROM (\n  %s\n) _agg;", inner), nil
	default:
		return fmt.Sprintf("SELECT %s(*) FROM (\n  %s\n) _agg;", agg.Func, inner), nil
	}
}

// compileShapeField compiles one field in a shape.
// Returns (column expression, lateral subquery string, error).
func (c *compiler) compileShapeField(f *aql.ShapeField, parentType *asl.ResolvedType, parentAlias string) (string, string, error) {
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
	var subCols []string
	if f.SubShape != nil {
		for _, sf := range f.SubShape.Fields {
			prop, ok := targetType.Properties[sf.Name]
			if !ok {
				return "", "", fmt.Errorf("type %q has no property %q", targetType.Name, sf.Name)
			}
			subCols = append(subCols, fmt.Sprintf("%s.%s AS %s", tAlias, prop.Column, sf.Name))
		}
	} else {
		for _, prop := range targetType.Properties {
			subCols = append(subCols, fmt.Sprintf("%s.%s", tAlias, prop.Column))
		}
	}

	if link.IsMulti {
		// Multi-link → a correlated json_agg scalar subquery. The junction table
		// has one FK column per side named after the referenced table (see
		// generateJunctionTable): targetType.Table (e.g. "user") and
		// parentType.Table (e.g. "project"). No LATERAL is needed.
		var inner string
		if link.JunctionTable != "" {
			jAlias := "jt_" + f.Name
			joinField := link.JoinField
			if joinField == "" {
				joinField = "id"
			}
			inner = fmt.Sprintf(
				"SELECT %s FROM \"%s\" %s JOIN \"%s\" %s ON %s.%s = %s.%s WHERE %s.%s = %s.id",
				strings.Join(subCols, ", "),
				link.JunctionTable, jAlias,
				targetType.Table, tAlias,
				tAlias, joinField, jAlias, targetType.Table,
				jAlias, parentType.Table, parentAlias,
			)
		} else {
			// Direct FK on the target side (rare for multi).
			inner = fmt.Sprintf(
				"SELECT %s FROM \"%s\" %s WHERE %s.%s = %s.id",
				strings.Join(subCols, ", "),
				targetType.Table, tAlias,
				tAlias, link.JoinColumn, parentAlias,
			)
		}

		col := fmt.Sprintf(
			"(SELECT COALESCE(json_agg(row_to_json(%s_sub)), '[]') FROM (%s) %s_sub) AS %s",
			tAlias, inner, tAlias, f.Name,
		)
		return col, "", nil
	}

	// Single link → correlated scalar subquery.
	joinCond := fmt.Sprintf("%s.id = %s.%s", tAlias, parentAlias, link.JoinColumn)

	col := fmt.Sprintf(
		"(SELECT row_to_json(%s_sub) FROM (SELECT %s FROM \"%s\" %s WHERE %s LIMIT 1) %s_sub) AS %s",
		tAlias,
		strings.Join(subCols, ", "),
		targetType.Table, tAlias,
		joinCond,
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
		sqRT, err := c.resolveType(sq.TypeName)
		if err != nil {
			return "", "", err
		}
		sqAlias := tableAlias(sq.TypeName)

		// Build inner SELECT columns.
		var innerCols []string
		if sq.Shape != nil {
			for _, sf := range sq.Shape.Fields {
				col, _, err := c.compileShapeField(sf, sqRT, sqAlias)
				if err != nil {
					return "", "", err
				}
				innerCols = append(innerCols, col)
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
		if where != "" {
			innerSQL += " WHERE " + where
		}

		sub := sqAlias + "_" + f.Name + "_sub"
		col := fmt.Sprintf(`(SELECT json_agg(row_to_json(%s)) FROM (%s) %s) AS %s`, sub, innerSQL, sub, f.Name)
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
	return c.compileInsertBody(stmt.TypeName, stmt.Assignments, true)
}

func (c *compiler) compileInsertBody(typeName string, assignments []*aql.Assignment, topLevel bool) (string, error) {
	rt, err := c.resolveType(typeName)
	if err != nil {
		return "", err
	}

	var cols, vals []string
	var ctes []string

	for _, a := range assignments {
		// Check if this is a link assignment.
		if link, ok := rt.Links[a.Field]; ok {
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

	var sb strings.Builder
	if len(ctes) > 0 {
		sb.WriteString("WITH ")
		sb.WriteString(strings.Join(ctes, ", "))
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "INSERT INTO \"%s\" (%s)\nVALUES (%s)",
		rt.Table,
		strings.Join(cols, ", "),
		strings.Join(vals, ", "),
	)
	if topLevel {
		fmt.Fprintf(&sb, "\nRETURNING %s;", returningColumns(rt))
		return sb.String(), nil
	}
	sb.WriteString(" RETURNING id")
	return sb.String(), nil
}

// compileLinkAssignment compiles a link assignment. Returns (column, value, cteFrag, error).
// cteFrag is non-empty when a sub-insert CTE was generated.
func (c *compiler) compileLinkAssignment(a *aql.Assignment, link *asl.ResolvedLink, parentType *asl.ResolvedType) (string, string, string, error) {
	operand := a.Value.SoloPrimary()
	if operand == nil {
		return "", "", "", fmt.Errorf("link %q assignment must be a subquery or sub-insert", a.Field)
	}
	col := fmt.Sprintf("%q", link.JoinColumn)

	// (insert TypeName { ... }) → CTE
	if operand.SubInsert != nil {
		sub := operand.SubInsert
		innerSQL, err := c.compileInsertBody(sub.TypeName, sub.Assignments, false)
		if err != nil {
			return "", "", "", fmt.Errorf("link %q sub-insert: %w", a.Field, err)
		}
		cteAlias := "_ins_" + link.JoinColumn
		cteFrag := fmt.Sprintf("%s AS (%s)", cteAlias, innerSQL)
		val := fmt.Sprintf("(SELECT id FROM %s)", cteAlias)
		return col, val, cteFrag, nil
	}

	// (select TypeName filter ...) → scalar subquery
	if operand.SubQuery == nil {
		return "", "", "", fmt.Errorf("link %q assignment must be a subquery (select ...) or sub-insert (insert ...)", a.Field)
	}
	sub := operand.SubQuery

	targetType, err := c.resolveType(link.TargetType)
	if err != nil {
		return "", "", "", err
	}
	alias := tableAlias(link.TargetType)

	var whereClause string
	if sub.Filter != nil {
		where, err := c.compileExpr(sub.Filter.Expr, alias, targetType)
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

// compileSubQuery compiles a (select ...) subquery used as a scalar expression.
// compileSubQuery compiles a scalar subquery. By default it projects the row's
// id; a non-empty projectField selects that property (or link FK) instead —
// e.g. (select Org filter .id = $id).slug.
func (c *compiler) compileSubQuery(body *aql.SelectBody, projectField string) (string, error) {
	rt, err := c.resolveType(body.TypeName)
	if err != nil {
		return "", err
	}
	alias := tableAlias(body.TypeName)

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
		where, err := c.compileExpr(body.Filter.Expr, alias, rt)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, " WHERE %s", where)
	}
	sb.WriteString(" LIMIT 1")
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
	alias := tableAlias(stmt.TypeName)

	var sets []string
	for _, a := range stmt.Assignments {
		prop, ok := rt.Properties[a.Field]
		if !ok {
			return "", fmt.Errorf("type %q has no property %q", stmt.TypeName, a.Field)
		}
		val, err := c.compileExpr(a.Value, alias, rt)
		if err != nil {
			return "", err
		}
		inferAssignmentParamType(c.params, a.Value, sqlToAQLType(prop.SQLType), prop.EnumType)
		sets = append(sets, fmt.Sprintf("%s = %s", prop.Column, val))
	}

	var sb strings.Builder
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

// ─────────────────────────────────────────────────────────────
// DELETE
// ─────────────────────────────────────────────────────────────

func (c *compiler) compileDelete(stmt *aql.DeleteStmt) (string, error) {
	rt, err := c.resolveType(stmt.TypeName)
	if err != nil {
		return "", err
	}
	alias := tableAlias(stmt.TypeName)

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

	arms := make([]string, 0, len(expr.Rest)+1)
	for _, a := range append([]*aql.AndExpr{expr.Left}, expr.Rest...) {
		sql, err := c.compileAndExpr(a, alias, rt)
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

func (c *compiler) compileAndExpr(and *aql.AndExpr, alias string, rt *asl.ResolvedType) (string, error) {
	if and == nil {
		return "", nil
	}

	parts := make([]string, 0, len(and.Rest)+1)
	for _, cmp := range append([]*aql.Cmp{and.Left}, and.Rest...) {
		sql, err := c.compileCmp(cmp, alias, rt)
		if err != nil {
			return "", err
		}
		parts = append(parts, sql)
	}
	return strings.Join(parts, " AND "), nil
}

// compileCmp compiles a single comparison. Param-type inference and the
// optional-param null guard live here, per comparison: a `$name?` guard must
// only relax its own comparison, never a whole conjunction.
func (c *compiler) compileCmp(cmp *aql.Cmp, alias string, rt *asl.ResolvedType) (string, error) {
	if cmp == nil {
		return "", nil
	}

	left, err := c.compilePrimary(cmp.Left, alias, rt)
	if err != nil {
		return "", err
	}

	if cmp.Op == "" {
		return left, nil
	}

	right, err := c.compilePrimary(cmp.Right, alias, rt)
	if err != nil {
		return "", err
	}

	// Infer param types from the opposite side of a comparison.
	if rt != nil {
		inferFilterParamType(c.params, cmp.Left, cmp.Right, rt)
		inferFilterParamType(c.params, cmp.Right, cmp.Left, rt)
	}

	// Null-coalesce ($x ?? .field) is a function, not an infix operator: emit
	// COALESCE(left, right). A param operand is cast to the SQL type of the
	// opposite operand so its type is determinable when the value is null.
	if cmp.Op == "??" {
		lc := left + c.paramCastSuffix(cmp.Left, cmp.Right, rt)
		rc := right + c.paramCastSuffix(cmp.Right, cmp.Left, rt)
		return fmt.Sprintf("COALESCE(%s, %s)", lc, rc), nil
	}

	result := fmt.Sprintf("%s %s %s", left, cmp.Op, right)

	// Optional params ($name?) make the comparison a no-op when the value is
	// null, so an omitted filter matches all rows. The standalone `$N IS NULL`
	// occurrence carries no type, so cast it to the SQL type of the column it's
	// compared against — otherwise Postgres can't determine the parameter type
	// when the value is null (42P08). Casting to the column type keeps the cast
	// consistent with the comparison (avoiding e.g. a str-vs-uuid conflict).
	for _, operand := range []*aql.Primary{cmp.Left, cmp.Right} {
		if operand != nil && operand.Param != nil && operand.Param.Optional {
			ph := c.params.add(operand.Param.Name, "")
			other := cmp.Right
			if operand == cmp.Right {
				other = cmp.Left
			}
			result = fmt.Sprintf("(%s%s IS NULL OR %s)", ph, c.paramCastSuffix(operand, other, rt), result)
		}
	}

	return result, nil
}

func (c *compiler) compilePrimary(p *aql.Primary, alias string, rt *asl.ResolvedType) (string, error) {
	if p == nil {
		return "", fmt.Errorf("nil primary expression")
	}

	switch {
	case p.SubQuery != nil:
		return c.compileSubQuery(p.SubQuery, p.SubQueryField)

	case p.SubInsert != nil:
		// (insert TypeName { ... }) used as a scalar — compile as a subquery returning id.
		sql, err := c.compileInsertBody(p.SubInsert.TypeName, p.SubInsert.Assignments, false)
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
	case p.QualifiedIdent != nil:
		qi := p.QualifiedIdent
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
		return *p.Ident, nil
	}

	return "", fmt.Errorf("empty primary expression")
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

	// Multi-step: .author.email → requires resolving link then property.
	// For now, emit a simple subquery.
	linkName := path.Steps[0]
	link, ok := rt.Links[linkName]
	if !ok {
		return "", fmt.Errorf("type %q has no link %q", rt.Name, linkName)
	}

	targetType, err := c.resolveType(link.TargetType)
	if err != nil {
		return "", err
	}

	tAlias := tableAlias(link.TargetType)
	remaining := path.Steps[1:]

	// Optimization: .link.id → FK column directly, avoiding a correlated subquery
	// and alias conflicts when the outer query already uses the same alias.
	if len(remaining) == 1 && remaining[0] == "id" {
		return fmt.Sprintf("%s.%s", alias, link.JoinColumn), nil
	}

	subPath := &aql.PathExpr{Steps: remaining}
	subExpr, err := c.compilePath(subPath, tAlias, targetType)
	if err != nil {
		return "", err
	}

	joinCond := link.JoinColumn
	if joinCond == "" {
		joinCond = strings.ToLower(linkName) + "_id"
	}

	return fmt.Sprintf(
		"(SELECT %s FROM \"%s\" %s WHERE %s.id = %s.%s LIMIT 1)",
		subExpr, targetType.Table, tAlias, tAlias, alias, joinCond,
	), nil
}

func (c *compiler) compileFuncCall(fc *aql.FuncCall, alias string, rt *asl.ResolvedType) (string, error) {
	var args []string
	for _, a := range fc.Args {
		s, err := c.compileExpr(a, alias, rt)
		if err != nil {
			return "", err
		}
		args = append(args, s)
	}
	return fmt.Sprintf("%s(%s)", fc.Name, strings.Join(args, ", ")), nil
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

// tableAlias returns a short lowercase alias for a type name.
func tableAlias(typeName string) string {
	if len(typeName) == 0 {
		return "t"
	}
	return strings.ToLower(string(typeName[0]))
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

// sqlToAQLType maps a SQL type string back to an AQL type name.
func sqlToAQLType(sqlType string) string {
	switch sqlType {
	case "TEXT":             return "str"
	case "SMALLINT":         return "int16"
	case "INTEGER":          return "int32"
	case "BIGINT":           return "int64"
	case "REAL":             return "float32"
	case "DOUBLE PRECISION": return "float64"
	case "BOOLEAN":          return "bool"
	case "UUID":             return "uuid"
	case "TIMESTAMPTZ":      return "datetime"
	case "DATE":             return "date"
	case "TIME":             return "time"
	case "JSONB":            return "json"
	case "BYTEA":            return "bytes"
	case "NUMERIC":          return "decimal"
	default:                 return ""
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
func inferFilterParamType(params *paramCollector, maybePath, maybeParam *aql.Primary, rt *asl.ResolvedType) {
	if maybePath == nil || maybeParam == nil || maybeParam.Param == nil {
		return
	}
	if maybePath.Path != nil && len(maybePath.Path.Steps) == 1 {
		if prop, ok := rt.Properties[maybePath.Path.Steps[0]]; ok {
			params.setType(maybeParam.Param.Name, sqlToAQLType(prop.SQLType))
			if prop.EnumType != "" {
				params.setEnumType(maybeParam.Param.Name, prop.EnumType)
			}
		}
	}
}

// filterOperandSQLType returns the SQL type of a single-step path operand — a
// scalar property's SQL type, or UUID for a link's FK column. Used to cast an
// optional param's `IS NULL` check so its type is known even when null.
func filterOperandSQLType(p *aql.Primary, rt *asl.ResolvedType) string {
	if p == nil || rt == nil || p.Path == nil || len(p.Path.Steps) != 1 {
		return ""
	}
	name := p.Path.Steps[0]
	if prop, ok := rt.Properties[name]; ok {
		return prop.SQLType
	}
	if _, ok := rt.Links[name]; ok {
		return "UUID" // FK columns reference the target's uuid id
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
	if t := filterOperandSQLType(other, rt); t != "" {
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
