package compiler

import (
	"fmt"
	"strings"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

// withCTEPrefix namespaces the CTE that backs a with-binding.
//
// The prefix is not cosmetic. Postgres resolves a CTE name ahead of a table of
// the same name for the whole statement, and does so regardless of quoting — so
// a binding written as `business := (select Business ...)` emitted as a CTE
// named "business" would shadow the business table everywhere else in the
// query, silently changing what an unrelated `FROM "business" b` reads. Names
// starting with `_with_` cannot collide with a generated table name.
const withCTEPrefix = "_with_"

// withBinding is one resolved entry of a `with (...)` block: the name as
// written, the CTE backing it, the type its rows come from (so `name.field` can
// be resolved), whether it was declared `multi` (a set rather than a row), and
// the optional set of fields explicitly selected by a { shape } (nil = all).
type withBinding struct {
	name            string
	cte             string
	rt              *asl.ResolvedType
	multi           bool
	projectedFields map[string]bool // nil = all fields; non-nil = shape-restricted
}

// compileWith lowers a with-block to CTEs, recording each binding so later
// expression compilation can resolve references to it. It runs before the
// statement body so parameters inside the bindings are numbered in the order
// they appear in the emitted SQL.
func (c *compiler) compileWith(w *aql.WithBlock) error {
	if w == nil {
		return nil
	}
	if c.trig != nil {
		return fmt.Errorf("a `with` block is not supported in a trigger or function body (there is no host statement to carry the CTE)")
	}
	c.withs = make(map[string]*withBinding, len(w.Bindings))

	for _, b := range w.Bindings {
		if _, dup := c.withs[b.Name]; dup {
			return fmt.Errorf("duplicate binding %q in `with` block", b.Name)
		}
		body, multi, err := withSubQuery(b)
		if err != nil {
			return err
		}
		rt, err := c.resolveType(body.TypeName)
		if err != nil {
			return fmt.Errorf("binding %q: %w", b.Name, err)
		}

		bind := &withBinding{name: b.Name, cte: withCTEPrefix + b.Name, rt: rt, multi: multi}
		sql, err := c.compileWithBody(bind, body)
		if err != nil {
			return err
		}
		// Register only after the body compiles, so a binding cannot reference
		// itself; earlier bindings are already visible, matching SQL's WITH.
		c.withs[b.Name] = bind
		c.withCTEs = append(c.withCTEs, fmt.Sprintf("%s AS (\n  %s\n)", bind.cte, sql))
	}
	return nil
}

// withSubQuery validates that a binding's value is a plain sub-select and
// returns its body along with whether it was declared `multi`.
func withSubQuery(b *aql.WithBinding) (*aql.SelectBody, bool, error) {
	p := b.Value.SoloPrimary()
	if p == nil || p.SubQuery == nil {
		return nil, false, fmt.Errorf("binding %q must be a (select ...) or (multi select ...)", b.Name)
	}
	if p.SubQueryField != "" {
		return nil, false, fmt.Errorf("binding %q: a with-binding binds the whole row, so it can't project .%s; project at the use site instead", b.Name, p.SubQueryField)
	}
	if p.Cast != "" {
		return nil, false, fmt.Errorf("binding %q: a with-binding binds rows, which can't be cast to <%s>", b.Name, p.Cast)
	}
	if p.SubQuery.AggFunc != nil {
		return nil, false, fmt.Errorf("binding %q: an aggregate can't be bound in a `with` block; use it directly in the query", b.Name)
	}
	return p.SubQuery, p.SubQueryMulti, nil
}

// compileWithBody builds the SELECT inside a binding's CTE.
//
// Without a shape, every scalar property and single-link FK column is projected
// under its AQL field name — the same expansion `select *` uses — so a
// `name.field` reference resolves by field name with no column mapping.
//
// With a `{ field, ... }` shape, only the listed fields are projected. A `*`
// anywhere in the shape expands back to all fields (same as no shape). Nested
// sub-shapes and computed fields are rejected — with-binding shapes may only
// name scalar properties and single-link FK columns.
//
// A plain binding is a single row (LIMIT 1, mirroring plain `select`); a `multi`
// binding is a set and honours an explicit limit/offset instead.
func (c *compiler) compileWithBody(bind *withBinding, body *aql.SelectBody) (string, error) {
	rt := bind.rt
	alias := c.newAlias(body.TypeName)

	var cols []string
	if body.Shape != nil {
		// Shape-restricted projection. Validate each field and collect only what
		// was named. A `*` anywhere reverts to the full projection.
		projected := make(map[string]bool, len(body.Shape.Fields))
		hasStar := false
		for _, sf := range body.Shape.Fields {
			if sf.Star {
				hasStar = true
				break
			}
			if sf.SubShape != nil {
				return "", fmt.Errorf("binding %q: nested shapes are not supported in a with-binding shape", bind.name)
			}
			if sf.Computed != nil {
				return "", fmt.Errorf("binding %q: computed fields (:=) are not supported in a with-binding shape", bind.name)
			}
			name := sf.Name
			if prop, ok := rt.Properties[name]; ok {
				cols = append(cols, fmt.Sprintf("%s.%s AS %s", alias, prop.Column, prop.Name))
				projected[name] = true
			} else if link, ok := rt.Links[name]; ok {
				if link.IsMulti {
					return "", fmt.Errorf("binding %q: %q is a multi-link and has no column on %s", bind.name, name, rt.Name)
				}
				cols = append(cols, fmt.Sprintf("%s.%s AS %s", alias, link.JoinColumn, link.JoinColumn))
				projected[name] = true
			} else {
				return "", fmt.Errorf("binding %q: type %q has no field %q", bind.name, rt.Name, name)
			}
		}
		if !hasStar {
			// Record shape restriction so withColumn can enforce it at reference sites.
			bind.projectedFields = projected
			var sb strings.Builder
			fmt.Fprintf(&sb, "SELECT %s FROM %q %s", strings.Join(cols, ", "), rt.Table, alias)
			if body.Filter != nil {
				where, err := c.compileExpr(body.Filter.Expr, alias, rt)
				if err != nil {
					return "", fmt.Errorf("binding %q: %w", bind.name, err)
				}
				fmt.Fprintf(&sb, " WHERE %s", where)
			}
			if len(body.OrderBy) > 0 {
				var parts []string
				for _, o := range body.OrderBy {
					expr, err := c.compileExpr(o.Expr, alias, rt)
					if err != nil {
						return "", fmt.Errorf("binding %q: %w", bind.name, err)
					}
					dir := strings.ToUpper(o.Dir)
					if dir == "" {
						dir = "ASC"
					}
					parts = append(parts, expr+" "+dir)
				}
				fmt.Fprintf(&sb, " ORDER BY %s", strings.Join(parts, ", "))
			}
			if !bind.multi {
				if body.Limit != nil || body.Offset != nil {
					return "", fmt.Errorf("binding %q: limit/offset require `multi select` (a plain binding is a single row)", bind.name)
				}
				sb.WriteString(" LIMIT 1")
				return sb.String(), nil
			}
			if body.Limit != nil {
				limit, err := c.compileExpr(body.Limit, alias, rt)
				if err != nil {
					return "", fmt.Errorf("binding %q: %w", bind.name, err)
				}
				fmt.Fprintf(&sb, " LIMIT %s", limit)
			}
			if body.Offset != nil {
				offset, err := c.compileExpr(body.Offset, alias, rt)
				if err != nil {
					return "", fmt.Errorf("binding %q: %w", bind.name, err)
				}
				fmt.Fprintf(&sb, " OFFSET %s", offset)
			}
			return sb.String(), nil
		}
		// hasStar: fall through to full projection below.
	}

	// No shape (or `*` in shape): project all scalar props + single-link FK columns.
	cols = nil
	for _, prop := range sortedProps(rt) {
		cols = append(cols, fmt.Sprintf("%s.%s AS %s", alias, prop.Column, prop.Name))
	}
	for _, link := range sortedSingleLinks(rt) {
		cols = append(cols, fmt.Sprintf("%s.%s AS %s", alias, link.JoinColumn, link.JoinColumn))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "SELECT %s FROM %q %s", strings.Join(cols, ", "), rt.Table, alias)

	if body.Filter != nil {
		where, err := c.compileExpr(body.Filter.Expr, alias, rt)
		if err != nil {
			return "", fmt.Errorf("binding %q: %w", bind.name, err)
		}
		fmt.Fprintf(&sb, " WHERE %s", where)
	}

	if len(body.OrderBy) > 0 {
		var parts []string
		for _, o := range body.OrderBy {
			expr, err := c.compileExpr(o.Expr, alias, rt)
			if err != nil {
				return "", fmt.Errorf("binding %q: %w", bind.name, err)
			}
			dir := strings.ToUpper(o.Dir)
			if dir == "" {
				dir = "ASC"
			}
			parts = append(parts, expr+" "+dir)
		}
		fmt.Fprintf(&sb, " ORDER BY %s", strings.Join(parts, ", "))
	}

	if !bind.multi {
		if body.Limit != nil || body.Offset != nil {
			return "", fmt.Errorf("binding %q: limit/offset require `multi select` (a plain binding is a single row)", bind.name)
		}
		sb.WriteString(" LIMIT 1")
		return sb.String(), nil
	}
	if body.Limit != nil {
		limit, err := c.compileExpr(body.Limit, alias, rt)
		if err != nil {
			return "", fmt.Errorf("binding %q: %w", bind.name, err)
		}
		fmt.Fprintf(&sb, " LIMIT %s", limit)
	}
	if body.Offset != nil {
		offset, err := c.compileExpr(body.Offset, alias, rt)
		if err != nil {
			return "", fmt.Errorf("binding %q: %w", bind.name, err)
		}
		fmt.Fprintf(&sb, " OFFSET %s", offset)
	}
	return sb.String(), nil
}

// withPrefix returns the `WITH ...` clause for the collected bindings, or "" when
// there are none. extra holds CTEs contributed by the statement itself (an
// insert's sub-insert CTEs), which are emitted after the bindings so they may
// reference them.
func (c *compiler) withPrefix(extra ...string) string {
	all := append(append([]string{}, c.withCTEs...), extra...)
	if len(all) == 0 {
		return ""
	}
	return "WITH " + strings.Join(all, ", ") + "\n"
}

// lookupWith returns the binding a name refers to, if any. Bindings shadow type
// and enum names of the same spelling, so this is consulted first everywhere a
// bare or qualified identifier is resolved.
func (c *compiler) lookupWith(name string) (*withBinding, bool) {
	b, ok := c.withs[name]
	return b, ok
}

// withRef lowers a reference to a binding's column to a scalar subquery over its
// CTE. field may be empty, meaning the row's id — which is what makes a bare
// `business` usable as a value and in `business is not null`.
//
// A `multi` binding is a set, so it is only meaningful on the right of `in`;
// compileCmp handles that case and this reports a usable error for every other
// position rather than letting Postgres fail at run time with "more than one row
// returned by a subquery".
func (c *compiler) withRef(b *withBinding, field string) (string, error) {
	col, err := withColumn(b, field)
	if err != nil {
		return "", err
	}
	if b.multi {
		return "", fmt.Errorf("binding %q is a `multi select` (a set, not a value); use it on the right of `in`, e.g. `.sender_id in %s.%s`", b.name, b.name, col)
	}
	return c.withSetRef(b, field)
}

// withSetRef lowers a binding reference without the single-row check, for the
// membership position where a set is exactly what is wanted.
func (c *compiler) withSetRef(b *withBinding, field string) (string, error) {
	return c.withSetRefCast(b, field, "")
}

// withSetRefCast is like withSetRef but applies an optional AQL type cast to
// the projected column. When cast is non-empty (e.g. "str") the subquery
// becomes `(SELECT (_with_api_key.id)::TEXT FROM _with_api_key)`, which lets
// Postgres coerce the set values without requiring the user to rewrite their
// `with` binding.
func (c *compiler) withSetRefCast(b *withBinding, field string, cast string) (string, error) {
	col, err := withColumn(b, field)
	if err != nil {
		return "", err
	}
	if cast != "" {
		sqlType, err := c.annotSQLType(cast)
		if err != nil {
			return "", fmt.Errorf("binding %q: cast <%s> on set reference: %w", b.name, cast, err)
		}
		return fmt.Sprintf("(SELECT (%s.%s)::%s FROM %s)", b.cte, col, sqlType, b.cte), nil
	}
	return fmt.Sprintf("(SELECT %s.%s FROM %s)", b.cte, col, b.cte), nil
}

// withColumn resolves a field name to the column the binding's CTE projects it
// as. An empty field means the row's id. When the binding was created with a
// { shape }, only the listed fields were projected into the CTE; referencing
// any other field is rejected here rather than letting Postgres fail at runtime.
func withColumn(b *withBinding, field string) (string, error) {
	lookup := field
	if lookup == "" {
		lookup = "id"
	}
	// Shape-restriction check.
	if b.projectedFields != nil && !b.projectedFields[lookup] {
		if field == "" {
			return "", fmt.Errorf("binding %q: field \"id\" was not included in the { shape }; add it to use the binding as a value", b.name)
		}
		return "", fmt.Errorf("binding %q: field %q was not included in the { shape }; add it or remove the shape restriction", b.name, field)
	}
	if field == "" {
		return "id", nil
	}
	if prop, ok := b.rt.Properties[field]; ok {
		return prop.Name, nil
	}
	if link, ok := b.rt.Links[field]; ok {
		if link.IsMulti {
			return "", fmt.Errorf("binding %q: %q is a multi-link and has no column on %s", b.name, field, b.rt.Name)
		}
		return link.JoinColumn, nil
	}
	return "", fmt.Errorf("binding %q: type %q has no field %q", b.name, b.rt.Name, field)
}

// withOperand reports whether an operand references a with-binding, returning
// the binding and the field named (empty for a bare reference, meaning id).
func (c *compiler) withOperand(p *aql.Primary) (*withBinding, string, bool) {
	if p == nil {
		return nil, "", false
	}
	if p.QualifiedIdent != nil {
		if b, ok := c.lookupWith(p.QualifiedIdent.TypeName); ok {
			return b, p.QualifiedIdent.Field, true
		}
	}
	if p.Ident != nil {
		if b, ok := c.lookupWith(*p.Ident); ok {
			return b, "", true
		}
	}
	return nil, "", false
}

// inferWithParamType types a param compared against a with-binding field.
// inferFilterParamType can't reach this case: the binding side of the
// comparison is a QualifiedIdent, not a path on the row being scanned.
func (c *compiler) inferWithParamType(maybeBinding, maybeParam *aql.Primary) {
	if maybeParam == nil || maybeParam.Param == nil {
		return
	}
	aqlType, enumType, ok := c.withParamType(maybeBinding)
	if !ok || aqlType == "" {
		return
	}
	c.params.setType(maybeParam.Param.Name, aqlType)
	if enumType != "" {
		c.params.setEnumType(maybeParam.Param.Name, enumType)
	}
}

// withParamType returns the AQL type (and enum type) of a binding field, so a
// param compared against `business.id` is typed instead of reaching codegen
// unknown. Returns ("", "", false) when p is not a reference to a binding field.
func (c *compiler) withParamType(p *aql.Primary) (string, string, bool) {
	if p == nil || p.QualifiedIdent == nil {
		return "", "", false
	}
	b, ok := c.lookupWith(p.QualifiedIdent.TypeName)
	if !ok {
		return "", "", false
	}
	if prop, ok := b.rt.Properties[p.QualifiedIdent.Field]; ok {
		return sqlToAQLType(prop.SQLType), prop.EnumType, true
	}
	if link, ok := b.rt.Links[p.QualifiedIdent.Field]; ok && !link.IsMulti {
		return "uuid", "", true
	}
	return "", "", false
}
