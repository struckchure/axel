package compiler

import (
	"fmt"
	"strings"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

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
		for _, f := range body.Shape.Fields {
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
		// No shape → select all scalar properties.
		for _, prop := range rt.Properties {
			cols = append(cols, fmt.Sprintf("%s.%s", alias, prop.Column))
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

	// LIMIT
	if body.Limit != nil {
		limit, err := c.compileExpr(body.Limit, alias, rt)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "\nLIMIT %s", limit)
	}

	// OFFSET
	if body.Offset != nil {
		offset, err := c.compileExpr(body.Offset, alias, rt)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "\nOFFSET %s", offset)
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
		return c.compileLinkField(f, link, parentAlias)
	}

	return "", "", fmt.Errorf("type %q has no field %q", parentType.Name, f.Name)
}

func (c *compiler) compileLinkField(f *aql.ShapeField, link *asl.ResolvedLink, parentAlias string) (string, string, error) {
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
		// Multi-link → json_agg over junction table join.
		var inner string
		if link.JunctionTable != "" {
			jAlias := "jt_" + f.Name
			joinField := link.JoinField
			if joinField == "" {
				joinField = "id"
			}
			inner = fmt.Sprintf(
				"SELECT %s\n      FROM \"%s\" %s\n      JOIN \"%s\" %s ON %s.id = %s.%s_id\n      WHERE %s.%s_id = %s.id",
				strings.Join(subCols, ", "),
				link.JunctionTable, jAlias,
				targetType.Table, tAlias,
				tAlias, jAlias, strings.ToLower(link.TargetType),
				jAlias, strings.ToLower(parentAlias), parentAlias,
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

		lateral := fmt.Sprintf(
			"    LATERAL (\n      SELECT COALESCE(json_agg(row_to_json(%s_sub)), '[]')\n      FROM (\n        %s\n      ) %s_sub\n    ) AS %s",
			tAlias, inner, tAlias, f.Name,
		)
		// The SELECT column is a bare reference to the lateral subquery alias.
		return fmt.Sprintf("(SELECT COALESCE(json_agg(row_to_json(%s_sub)), '[]') FROM (SELECT %s FROM \"%s\" %s WHERE %s.%s_id = %s.id) %s_sub) AS %s",
			tAlias,
			strings.Join(subCols, ", "),
			targetType.Table, tAlias,
			tAlias, strings.ToLower(link.TargetType), parentAlias,
			tAlias,
			f.Name,
		), lateral, nil
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
		inferAssignmentParamType(c.params, a.Value, sqlToAQLType(prop.SQLType))
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
		sb.WriteString("\nRETURNING *;")
		return "BEGIN;\n" + sb.String() + "\nCOMMIT;", nil
	}
	sb.WriteString(" RETURNING id")
	return sb.String(), nil
}

// compileLinkAssignment compiles a link assignment. Returns (column, value, cteFrag, error).
// cteFrag is non-empty when a sub-insert CTE was generated.
func (c *compiler) compileLinkAssignment(a *aql.Assignment, link *asl.ResolvedLink, parentType *asl.ResolvedType) (string, string, string, error) {
	if a.Value.Left == nil {
		return "", "", "", fmt.Errorf("link %q assignment must be a subquery or sub-insert", a.Field)
	}
	col := fmt.Sprintf("%q", link.JoinColumn)

	// (insert TypeName { ... }) → CTE
	if a.Value.Left.SubInsert != nil {
		sub := a.Value.Left.SubInsert
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
	if a.Value.Left.SubQuery == nil {
		return "", "", "", fmt.Errorf("link %q assignment must be a subquery (select ...) or sub-insert (insert ...)", a.Field)
	}
	sub := a.Value.Left.SubQuery

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
func (c *compiler) compileSubQuery(body *aql.SelectBody) (string, error) {
	rt, err := c.resolveType(body.TypeName)
	if err != nil {
		return "", err
	}
	alias := tableAlias(body.TypeName)

	var sb strings.Builder
	fmt.Fprintf(&sb, "SELECT %s.id FROM \"%s\" %s", alias, rt.Table, alias)

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
		inferAssignmentParamType(c.params, a.Value, sqlToAQLType(prop.SQLType))
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
	sb.WriteString("\nRETURNING *;")
	return "BEGIN;\n" + sb.String() + "\nCOMMIT;", nil
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
	return "BEGIN;\n" + sb.String() + "\nCOMMIT;", nil
}

// ─────────────────────────────────────────────────────────────
// EXPRESSION COMPILATION
// ─────────────────────────────────────────────────────────────

func (c *compiler) compileExpr(expr *aql.Expr, alias string, rt *asl.ResolvedType) (string, error) {
	if expr == nil {
		return "", nil
	}

	left, err := c.compilePrimary(expr.Left, alias, rt)
	if err != nil {
		return "", err
	}

	if expr.Op == "" {
		return left, nil
	}

	right, err := c.compilePrimary(expr.Right, alias, rt)
	if err != nil {
		return "", err
	}

	// Infer param types from the opposite side of a comparison.
	if rt != nil {
		inferFilterParamType(c.params, expr.Left, expr.Right, rt)
		inferFilterParamType(c.params, expr.Right, expr.Left, rt)
	}

	op := mapOp(expr.Op)
	return fmt.Sprintf("%s %s %s", left, op, right), nil
}

func (c *compiler) compilePrimary(p *aql.Primary, alias string, rt *asl.ResolvedType) (string, error) {
	if p == nil {
		return "", fmt.Errorf("nil primary expression")
	}

	switch {
	case p.SubQuery != nil:
		return c.compileSubQuery(p.SubQuery)

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
		return c.params.add(*p.Param, ""), nil

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

// mapOp maps AQL operators to SQL operators.
func mapOp(op string) string {
	switch op {
	case "and":
		return "AND"
	case "or":
		return "OR"
	case "??":
		return "IS NOT DISTINCT FROM" // coalesce-like; actually use COALESCE in practice
	default:
		return op
	}
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

// inferAssignmentParamType sets the param type when an assignment value is a bare $param.
func inferAssignmentParamType(params *paramCollector, val *aql.Expr, aqlType string) {
	if val != nil && val.Op == "" && val.Left != nil && val.Left.Param != nil {
		params.setType(*val.Left.Param, aqlType)
	}
}

// inferFilterParamType sets a param's type when paired with a path on the other side of a binary op.
func inferFilterParamType(params *paramCollector, maybePath, maybeParam *aql.Primary, rt *asl.ResolvedType) {
	if maybePath == nil || maybeParam == nil || maybeParam.Param == nil {
		return
	}
	if maybePath.Path != nil && len(maybePath.Path.Steps) == 1 {
		if prop, ok := rt.Properties[maybePath.Path.Steps[0]]; ok {
			params.setType(*maybeParam.Param, sqlToAQLType(prop.SQLType))
		}
	}
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
