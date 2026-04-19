package asl

import (
	"fmt"
	"strings"
)

// builtinTypes maps ASL scalar type names to their SQL equivalents.
var builtinTypes = map[string]string{
	"str":      "TEXT",
	"int16":    "SMALLINT",
	"int32":    "INTEGER",
	"int64":    "BIGINT",
	"float32":  "REAL",
	"float64":  "DOUBLE PRECISION",
	"bool":     "BOOLEAN",
	"uuid":     "UUID",
	"datetime": "TIMESTAMPTZ",
	"date":     "DATE",
	"time":     "TIME",
	"json":     "JSONB",
	"bytes":    "BYTEA",
	"decimal":  "NUMERIC",
}

// Resolver builds a SchemaIR from a parsed SourceFile.
type Resolver struct{}

// Resolve resolves a parsed SourceFile into a SchemaIR.
func (r *Resolver) Resolve(src *SourceFile) (*SchemaIR, error) {
	ir := &SchemaIR{
		ScalarTypes: make(map[string]*ResolvedScalar),
		EnumTypes:   make(map[string]*ResolvedEnum),
		ObjectTypes: make(map[string]*ResolvedType),
	}

	// Pass 1: register scalar types and enum types.
	for _, def := range src.Definitions {
		switch {
		case def.ScalarType != nil:
			s := def.ScalarType
			sqlType, err := r.resolveBaseType(s.Extends, ir)
			if err != nil {
				return nil, fmt.Errorf("scalar type %q: %w", s.Name, err)
			}
			ir.ScalarTypes[s.Name] = &ResolvedScalar{
				Name:    s.Name,
				Base:    s.Extends,
				SQLType: sqlType,
			}

		case def.EnumType != nil:
			e := def.EnumType
			ir.EnumTypes[e.Name] = &ResolvedEnum{
				Name:   e.Name,
				Values: e.Values,
			}
		}
	}

	// Pass 2: register object types (abstract and concrete) without members.
	for _, def := range src.Definitions {
		if def.TypeDef == nil {
			continue
		}
		t := def.TypeDef
		rt := &ResolvedType{
			Name:       t.Name,
			IsAbstract: t.Abstract,
			Table:      toSnakeCase(t.Name),
			Properties: make(map[string]*ResolvedProp),
			Links:      make(map[string]*ResolvedLink),
			Computed:   make(map[string]*ResolvedComputed),
		}
		if t.Abstract {
			rt.Table = "" // abstract types have no table
		}
		ir.ObjectTypes[t.Name] = rt
	}

	// Pass 3: resolve members for each type (with inheritance).
	for _, def := range src.Definitions {
		if def.TypeDef == nil {
			continue
		}
		t := def.TypeDef
		rt := ir.ObjectTypes[t.Name]

		// Inherit from parent types first.
		for _, parentName := range t.Extending {
			parent, ok := ir.ObjectTypes[parentName]
			if !ok {
				return nil, fmt.Errorf("type %q extends unknown type %q", t.Name, parentName)
			}
			for k, v := range parent.Properties {
				rt.Properties[k] = v
			}
			for k, v := range parent.Links {
				rt.Links[k] = v
			}
			for k, v := range parent.Computed {
				rt.Computed[k] = v
			}
			rt.Indexes = append(rt.Indexes, parent.Indexes...)
		}

		// Resolve own members.
		for _, m := range t.Members {
			if err := r.resolveMember(m, rt, ir); err != nil {
				return nil, fmt.Errorf("type %q: %w", t.Name, err)
			}
		}
	}

	return ir, nil
}

func (r *Resolver) resolveMember(m *Member, rt *ResolvedType, ir *SchemaIR) error {
	switch {
	case m.Computed != nil:
		rt.Computed[m.Computed.Name] = &ResolvedComputed{
			Name: m.Computed.Name,
			Expr: strings.Join(m.Computed.Parts, ""),
		}

	case m.Index != nil:
		idx := &ResolvedIndex{}
		for _, f := range m.Index.Fields {
			idx.Columns = append(idx.Columns, toSnakeCase(f))
		}
		rt.Indexes = append(rt.Indexes, idx)

	case m.Field != nil:
		if err := r.resolveField(m.Field, rt, ir); err != nil {
			return err
		}
	}
	return nil
}

func (r *Resolver) resolveField(f *FieldDecl, rt *ResolvedType, ir *SchemaIR) error {
	// Determine if this is a link or a property.
	isLink := false
	var targetTypeName string
	var linkField string // for old-style "on field"

	if f.TypeSpec == nil {
		return fmt.Errorf("field %q has no type annotation", f.Name)
	}

	if f.TypeSpec.PropType != nil {
		typeName := *f.TypeSpec.PropType

		// If the link keyword is present or the target is a known object type → link.
		if f.LinkKeyword {
			isLink = true
			targetTypeName = typeName
		} else if _, ok := ir.ObjectTypes[typeName]; ok {
			isLink = true
			targetTypeName = typeName
		}
		// Otherwise it's a property (builtin, scalar alias, or enum).
	}

	// Extract on-clause from body.
	if f.Body != nil {
		for _, item := range f.Body.Items {
			if item.OnClause != nil {
				linkField = item.OnClause.Field
			}
		}
	}

	if isLink {
		return r.resolveLink(f, rt, ir, targetTypeName, linkField)
	}
	return r.resolveProp(f, rt, ir)
}

func (r *Resolver) resolveProp(f *FieldDecl, rt *ResolvedType, ir *SchemaIR) error {
	typeName := *f.TypeSpec.PropType

	sqlType, err := r.resolveBaseType(typeName, ir)
	if err != nil {
		return fmt.Errorf("property %q: %w", f.Name, err)
	}

	prop := &ResolvedProp{
		Name:       f.Name,
		Column:     toSnakeCase(f.Name),
		SQLType:    sqlType,
		IsRequired: f.Required,
		IsMulti:    f.Multi,
	}

	// Extract default and constraints from body.
	if f.Body != nil {
		for _, item := range f.Body.Items {
			switch {
			case item.Default != nil:
				prop.Default = resolveDefault(item.Default, sqlType)
			case item.Constraint != nil:
				prop.Constraints = append(prop.Constraints, ResolvedConstraint{
					Name: item.Constraint.Name,
					Args: item.Constraint.Args,
				})
			}
		}
	}

	rt.Properties[f.Name] = prop
	return nil
}

func (r *Resolver) resolveLink(f *FieldDecl, rt *ResolvedType, ir *SchemaIR, targetType, joinField string) error {
	if _, ok := ir.ObjectTypes[targetType]; !ok {
		return fmt.Errorf("link %q references unknown type %q", f.Name, targetType)
	}

	link := &ResolvedLink{
		Name:       f.Name,
		TargetType: targetType,
		JoinField:  joinField,
		IsRequired: f.Required,
		IsMulti:    f.Multi,
	}

	if f.Multi {
		// Multi-link → junction table: source_linkname
		link.JunctionTable = fmt.Sprintf("%s_%s", toSnakeCase(rt.Name), toSnakeCase(f.Name))
	} else {
		// Single link → FK column: fieldname (matches migration SQL generator convention)
		link.JoinColumn = toSnakeCase(f.Name)
	}

	rt.Links[f.Name] = link
	return nil
}

// resolveBaseType resolves a type name to its SQL equivalent.
func (r *Resolver) resolveBaseType(typeName string, ir *SchemaIR) (string, error) {
	if sqlType, ok := builtinTypes[typeName]; ok {
		return sqlType, nil
	}
	if scalar, ok := ir.ScalarTypes[typeName]; ok {
		return scalar.SQLType, nil
	}
	if enum, ok := ir.EnumTypes[typeName]; ok {
		_ = enum
		return "TEXT", nil // enums stored as TEXT with CHECK constraint
	}
	return "", fmt.Errorf("unknown type %q", typeName)
}

// resolveDefault converts a DefaultDecl to a SQL DEFAULT expression.
func resolveDefault(d *DefaultDecl, sqlType string) string {
	switch {
	case d.NewFunc != nil:
		return mapFuncDefault(*d.NewFunc, sqlType)
	case d.NewLit != nil:
		return mapLitDefault(*d.NewLit, sqlType)
	case d.OldFunc != nil:
		return mapFuncDefault(*d.OldFunc, sqlType)
	case d.OldLit != nil:
		return mapLitDefault(*d.OldLit, sqlType)
	}
	return ""
}

func mapFuncDefault(name, sqlType string) string {
	switch name {
	case "gen_uuid", "gen_random_uuid":
		return "gen_random_uuid()"
	case "now", "datetime_current":
		return "now()"
	default:
		return name + "()"
	}
}

func mapLitDefault(lit, sqlType string) string {
	// Strip surrounding single-quotes from string literals.
	if strings.HasPrefix(lit, "'") && strings.HasSuffix(lit, "'") {
		return lit // already SQL-compatible single-quoted string
	}
	// Boolean literals.
	if lit == "true" || lit == "false" {
		return lit
	}
	// Numeric literals.
	return lit
}

// toSnakeCase converts CamelCase or mixed identifiers to snake_case.
func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result = append(result, '_')
			}
			result = append(result, r+32)
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}
