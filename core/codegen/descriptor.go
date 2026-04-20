package codegen

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

// SchemaDescriptor is a JSON-serializable snapshot of the resolved ASL schema.
type SchemaDescriptor struct {
	Scalars []ScalarDescriptor `json:"scalars"`
	Enums   []EnumDescriptor   `json:"enums"`
	Types   []TypeDescriptor   `json:"types"`
}

// TypeDescriptor describes one concrete or abstract object type.
type TypeDescriptor struct {
	Name       string               `json:"name"`
	Table      string               `json:"table"`
	IsAbstract bool                 `json:"is_abstract"`
	Extends    []string             `json:"extends,omitempty"`
	Properties []PropertyDescriptor `json:"properties,omitempty"`
	Links      []LinkDescriptor     `json:"links,omitempty"`
	Computed   []ComputedDescriptor `json:"computed,omitempty"`
	Indexes    []IndexDescriptor    `json:"indexes,omitempty"`
}

// PropertyDescriptor describes a scalar column property.
type PropertyDescriptor struct {
	Name        string                `json:"name"`
	Column      string                `json:"column"`
	AQLType     string                `json:"aql_type"`
	SQLType     string                `json:"sql_type"`
	IsRequired  bool                  `json:"is_required"`
	IsMulti     bool                  `json:"is_multi"`
	Default     string                `json:"default,omitempty"`
	Constraints []ConstraintDescriptor `json:"constraints,omitempty"`
}

// LinkDescriptor describes a foreign-key or junction-table relationship.
type LinkDescriptor struct {
	Name          string `json:"name"`
	TargetType    string `json:"target_type"`
	JoinColumn    string `json:"join_column,omitempty"`
	JunctionTable string `json:"junction_table,omitempty"`
	IsRequired    bool   `json:"is_required"`
	IsMulti       bool   `json:"is_multi"`
}

// EnumDescriptor describes an enum definition.
type EnumDescriptor struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// ScalarDescriptor describes a named scalar alias.
type ScalarDescriptor struct {
	Name    string `json:"name"`
	Base    string `json:"base"`
	SQLType string `json:"sql_type"`
}

// ComputedDescriptor describes a computed (non-stored) field.
type ComputedDescriptor struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

// IndexDescriptor describes a multi-column index.
type IndexDescriptor struct {
	Columns []string `json:"columns"`
}

// ConstraintDescriptor describes a field constraint.
type ConstraintDescriptor struct {
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
}

// QueryDescriptor describes one compiled AQL query file.
type QueryDescriptor struct {
	Name      string            `json:"name"`      // camelCase function name
	File      string            `json:"file"`      // source .aql file path
	SQL       string            `json:"sql"`       // compiled parameterized SQL
	Operation string            `json:"operation"` // "select", "insert", "update", "delete"
	Params    []ParamDescriptor `json:"params,omitempty"`
	Result    ResultDescriptor  `json:"result"`
}

// ParamDescriptor describes one named query parameter.
type ParamDescriptor struct {
	Name    string `json:"name"`
	AQLType string `json:"aql_type"`
	SQLPos  int    `json:"sql_pos"` // 1-based $N
}

// ResultDescriptor describes the shape of data returned by a query.
type ResultDescriptor struct {
	Fields     []ResultField `json:"fields,omitempty"`
	IsMultiple bool          `json:"is_multiple"` // true → []Row, false → *Row
	IsScalar   bool          `json:"is_scalar"`   // true → count/aggregate
}

// ResultField describes one field in a query result.
type ResultField struct {
	Name       string        `json:"name"`
	AQLType    string        `json:"aql_type,omitempty"`
	SQLType    string        `json:"sql_type,omitempty"`
	IsNullable bool          `json:"is_nullable"`
	IsMultiple bool          `json:"is_multiple"` // multi-link → json array
	TargetType string        `json:"target_type,omitempty"`
	SubFields  []ResultField `json:"sub_fields,omitempty"`
}

// FromSchemaIR converts a resolved SchemaIR into a SchemaDescriptor.
func FromSchemaIR(ir *asl.SchemaIR) SchemaDescriptor {
	desc := SchemaDescriptor{}

	// Scalars (sorted for determinism)
	scalarNames := make([]string, 0, len(ir.ScalarTypes))
	for n := range ir.ScalarTypes {
		scalarNames = append(scalarNames, n)
	}
	sort.Strings(scalarNames)
	for _, n := range scalarNames {
		s := ir.ScalarTypes[n]
		desc.Scalars = append(desc.Scalars, ScalarDescriptor{
			Name: s.Name, Base: s.Base, SQLType: s.SQLType,
		})
	}

	// Enums (sorted)
	enumNames := make([]string, 0, len(ir.EnumTypes))
	for n := range ir.EnumTypes {
		enumNames = append(enumNames, n)
	}
	sort.Strings(enumNames)
	for _, n := range enumNames {
		e := ir.EnumTypes[n]
		desc.Enums = append(desc.Enums, EnumDescriptor{Name: e.Name, Values: e.Values})
	}

	// Types (sorted)
	typeNames := make([]string, 0, len(ir.ObjectTypes))
	for n := range ir.ObjectTypes {
		typeNames = append(typeNames, n)
	}
	sort.Strings(typeNames)
	for _, n := range typeNames {
		t := ir.ObjectTypes[n]
		td := TypeDescriptor{
			Name:       t.Name,
			Table:      t.Table,
			IsAbstract: t.IsAbstract,
		}

		// Properties (sorted)
		propNames := make([]string, 0, len(t.Properties))
		for pn := range t.Properties {
			propNames = append(propNames, pn)
		}
		sort.Strings(propNames)
		for _, pn := range propNames {
			p := t.Properties[pn]
			pd := PropertyDescriptor{
				Name:       p.Name,
				Column:     p.Column,
				AQLType:    sqlTypeToAQLType(p.SQLType),
				SQLType:    p.SQLType,
				IsRequired: p.IsRequired,
				IsMulti:    p.IsMulti,
				Default:    p.Default,
			}
			for _, c := range p.Constraints {
				pd.Constraints = append(pd.Constraints, ConstraintDescriptor{Name: c.Name, Args: c.Args})
			}
			td.Properties = append(td.Properties, pd)
		}

		// Links (sorted)
		linkNames := make([]string, 0, len(t.Links))
		for ln := range t.Links {
			linkNames = append(linkNames, ln)
		}
		sort.Strings(linkNames)
		for _, ln := range linkNames {
			l := t.Links[ln]
			td.Links = append(td.Links, LinkDescriptor{
				Name:          l.Name,
				TargetType:    l.TargetType,
				JoinColumn:    l.JoinColumn,
				JunctionTable: l.JunctionTable,
				IsRequired:    l.IsRequired,
				IsMulti:       l.IsMulti,
			})
		}

		// Computed (sorted)
		compNames := make([]string, 0, len(t.Computed))
		for cn := range t.Computed {
			compNames = append(compNames, cn)
		}
		sort.Strings(compNames)
		for _, cn := range compNames {
			c := t.Computed[cn]
			td.Computed = append(td.Computed, ComputedDescriptor{Name: c.Name, Expr: c.Expr})
		}

		// Indexes
		for _, idx := range t.Indexes {
			td.Indexes = append(td.Indexes, IndexDescriptor{Columns: idx.Columns})
		}

		desc.Types = append(desc.Types, td)
	}

	return desc
}

// BuildQueryDescriptor converts a parsed+compiled AQL query into a QueryDescriptor.
func BuildQueryDescriptor(name, file string, stmt *aql.Statement, compiled *compiler.CompiledSQL, ir *asl.SchemaIR) (QueryDescriptor, error) {
	if name == "" {
		name = queryNameFromFile(file)
	}

	desc := QueryDescriptor{
		Name:  name,
		File:  file,
		SQL:   compiled.SQL,
	}

	// Params
	for i, p := range compiled.Params {
		desc.Params = append(desc.Params, ParamDescriptor{
			Name:    p.Name,
			AQLType: p.AQLType,
			SQLPos:  i + 1,
		})
	}

	// Operation + result shape
	switch {
	case stmt.Select != nil:
		desc.Operation = "select"
		body := stmt.Select.Body
		if body.AggFunc != nil {
			desc.Result.IsScalar = true
		} else {
			desc.Result.IsMultiple = true
			rt := ir.ObjectTypes[body.TypeName]
			if rt != nil && body.Shape != nil {
				desc.Result.Fields = buildShapeFields(body.Shape, rt, ir)
			} else if rt != nil {
				// No explicit shape — all properties
				desc.Result.Fields = allPropsAsFields(rt, ir)
			}
		}

	case stmt.Insert != nil:
		desc.Operation = "insert"
		rt := ir.ObjectTypes[stmt.Insert.TypeName]
		if rt != nil {
			desc.Result.Fields = allPropsAsFields(rt, ir)
		}

	case stmt.Update != nil:
		desc.Operation = "update"
		rt := ir.ObjectTypes[stmt.Update.TypeName]
		if rt != nil {
			desc.Result.Fields = allPropsAsFields(rt, ir)
		}

	case stmt.Delete != nil:
		desc.Operation = "delete"
	}

	return desc, nil
}

// buildShapeFields recursively resolves AQL shape fields against the schema.
func buildShapeFields(shape *aql.Shape, rt *asl.ResolvedType, ir *asl.SchemaIR) []ResultField {
	var fields []ResultField
	for _, sf := range shape.Fields {
		// Inline computed field: name := expr
		if sf.Computed != nil {
			if sf.Computed.Op == "" && sf.Computed.Left != nil && sf.Computed.Left.SubQuery != nil {
				sq := sf.Computed.Left.SubQuery
				targetRT := ir.ObjectTypes[sq.TypeName]
				var subFields []ResultField
				if targetRT != nil {
					if sq.Shape != nil {
						subFields = buildShapeFields(sq.Shape, targetRT, ir)
					} else {
						subFields = allPropsAsFields(targetRT, ir)
					}
				}
				fields = append(fields, ResultField{
					Name:       sf.Name,
					IsMultiple: true,
					IsNullable: true,
					TargetType: sq.TypeName,
					SubFields:  subFields,
				})
			} else {
				fields = append(fields, ResultField{
					Name:       sf.Name,
					AQLType:    "json",
					IsNullable: true,
				})
			}
			continue
		}

		if sf.SubShape != nil {
			// Link field with nested shape
			link, ok := rt.Links[sf.Name]
			if !ok {
				continue
			}
			targetRT := ir.ObjectTypes[link.TargetType]
			rf := ResultField{
				Name:       sf.Name,
				IsMultiple: link.IsMulti,
				IsNullable: !link.IsRequired,
				TargetType: link.TargetType,
			}
			if targetRT != nil {
				rf.SubFields = buildShapeFields(sf.SubShape, targetRT, ir)
			}
			fields = append(fields, rf)
		} else {
			// Scalar property
			if prop, ok := rt.Properties[sf.Name]; ok {
				fields = append(fields, ResultField{
					Name:       sf.Name,
					AQLType:    sqlTypeToAQLType(prop.SQLType),
					SQLType:    prop.SQLType,
					IsNullable: !prop.IsRequired,
				})
			} else if link, ok := rt.Links[sf.Name]; ok {
				// Link selected without sub-shape (just the FK value)
				fields = append(fields, ResultField{
					Name:       sf.Name,
					AQLType:    "uuid",
					SQLType:    "UUID",
					IsNullable: !link.IsRequired,
					IsMultiple: link.IsMulti,
					TargetType: link.TargetType,
				})
			}
		}
	}
	return fields
}

// allPropsAsFields returns all scalar properties of a type as ResultFields.
func allPropsAsFields(rt *asl.ResolvedType, _ *asl.SchemaIR) []ResultField {
	names := make([]string, 0, len(rt.Properties))
	for n := range rt.Properties {
		names = append(names, n)
	}
	sort.Strings(names)
	var fields []ResultField
	for _, n := range names {
		p := rt.Properties[n]
		fields = append(fields, ResultField{
			Name:       p.Name,
			AQLType:    sqlTypeToAQLType(p.SQLType),
			SQLType:    p.SQLType,
			IsNullable: !p.IsRequired,
		})
	}
	return fields
}

// queryNameFromFile converts a file path to a camelCase function name.
// Strips all extensions (e.g. "list_post.query.aql" → "listPost").
// Non-alphanumeric segments (dots, hyphens) are treated as word separators.
func queryNameFromFile(file string) string {
	base := filepath.Base(file)
	// Strip all extensions
	for ext := filepath.Ext(base); ext != ""; ext = filepath.Ext(base) {
		base = strings.TrimSuffix(base, ext)
	}
	// Replace hyphens and dots with underscores then split
	base = strings.NewReplacer("-", "_", ".", "_").Replace(base)
	parts := strings.Split(base, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 {
			parts[i] = strings.ToLower(p)
		} else {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, "")
}

// ToSchemaIR reconstructs an asl.SchemaIR from a SchemaDescriptor.
// This is the inverse of FromSchemaIR and is used by the generated runner to
// compile inline AQL queries at runtime.
func ToSchemaIR(sd SchemaDescriptor) *asl.SchemaIR {
	ir := &asl.SchemaIR{
		ScalarTypes: make(map[string]*asl.ResolvedScalar),
		EnumTypes:   make(map[string]*asl.ResolvedEnum),
		ObjectTypes: make(map[string]*asl.ResolvedType),
	}
	for _, s := range sd.Scalars {
		ir.ScalarTypes[s.Name] = &asl.ResolvedScalar{
			Name:    s.Name,
			Base:    s.Base,
			SQLType: s.SQLType,
		}
	}
	for _, e := range sd.Enums {
		ir.EnumTypes[e.Name] = &asl.ResolvedEnum{
			Name:   e.Name,
			Values: e.Values,
		}
	}
	for _, t := range sd.Types {
		rt := &asl.ResolvedType{
			Name:       t.Name,
			IsAbstract: t.IsAbstract,
			Table:      t.Table,
			Properties: make(map[string]*asl.ResolvedProp),
			Links:      make(map[string]*asl.ResolvedLink),
			Computed:   make(map[string]*asl.ResolvedComputed),
		}
		for _, p := range t.Properties {
			var constraints []asl.ResolvedConstraint
			for _, c := range p.Constraints {
				constraints = append(constraints, asl.ResolvedConstraint{Name: c.Name, Args: c.Args})
			}
			rt.Properties[p.Name] = &asl.ResolvedProp{
				Name:        p.Name,
				Column:      p.Column,
				SQLType:     p.SQLType,
				IsRequired:  p.IsRequired,
				IsMulti:     p.IsMulti,
				Default:     p.Default,
				Constraints: constraints,
			}
		}
		for _, l := range t.Links {
			rt.Links[l.Name] = &asl.ResolvedLink{
				Name:          l.Name,
				TargetType:    l.TargetType,
				JoinColumn:    l.JoinColumn,
				JunctionTable: l.JunctionTable,
				IsRequired:    l.IsRequired,
				IsMulti:       l.IsMulti,
			}
		}
		for _, c := range t.Computed {
			rt.Computed[c.Name] = &asl.ResolvedComputed{Name: c.Name, Expr: c.Expr}
		}
		for _, idx := range t.Indexes {
			rt.Indexes = append(rt.Indexes, &asl.ResolvedIndex{Columns: idx.Columns})
		}
		ir.ObjectTypes[t.Name] = rt
	}
	return ir
}

// sqlTypeToAQLType maps a SQL type string back to an AQL type name.
func sqlTypeToAQLType(sqlType string) string {
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
	case "JSONB":
		return "json"
	case "BYTEA":
		return "bytes"
	case "NUMERIC":
		return "decimal"
	default:
		return strings.ToLower(sqlType)
	}
}
