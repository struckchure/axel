package axel

import (
	"github.com/struckchure/axel/core/asl"
)

type Model struct {
	Name        string
	IsAbstract  bool
	Extends     string
	Fields      []Field
	Indexes     []Index
	Constraints []TypeConstraint
}

// Index is an index over one or more columns of a model.
type Index struct {
	Columns []string
}

// TypeConstraint is a type-level constraint spanning one or more columns
// (composite UNIQUE / PRIMARY KEY, or a length CHECK).
type TypeConstraint struct {
	Expression string
	Args       []string
	Columns    []string
}

type OnTarget struct {
	Name string
	Type string
}

type Field struct {
	Name        string
	Type        string
	IsRequired  bool
	IsMulti     bool
	Constraints []Constraint
	Default     string
	OnTarget    OnTarget // For links
	EnumType    string   // enum type name when the column is enum-backed
	EnumValues  []string // allowed enum values (drives the CHECK constraint)
}

type Constraint struct {
	Name string
	Args []string
}

// SchemaIRToModels converts a resolved asl.SchemaIR into the legacy []Model
// format consumed by the migration SQL generator.
//
// Inheritance is already flattened in SchemaIR, so all models are emitted with
// their full field set and no Extends value.
func SchemaIRToModels(ir *asl.SchemaIR) []Model {
	var models []Model

	for _, rt := range ir.ObjectTypes {
		model := Model{
			Name:       rt.Name,
			IsAbstract: rt.IsAbstract,
		}

		// Properties → scalar fields.
		for _, prop := range rt.Properties {
			f := Field{
				Name:       prop.Name,
				Type:       sqlTypeToASLType(prop.SQLType),
				IsRequired: prop.IsRequired,
				IsMulti:    prop.IsMulti,
				Default:    prop.Default,
			}
			for _, c := range prop.Constraints {
				f.Constraints = append(f.Constraints, Constraint{
					Name: c.Name,
					Args: c.Args,
				})
			}
			// Enum-backed property → carry enum identity and values for the CHECK.
			if prop.EnumType != "" {
				f.EnumType = prop.EnumType
				if enum, ok := ir.EnumTypes[prop.EnumType]; ok {
					f.EnumValues = append([]string(nil), enum.Values...)
				}
			}
			model.Fields = append(model.Fields, f)
		}

		// Links → FK fields.
		for _, link := range rt.Links {
			joinField := link.JoinField
			if joinField == "" {
				joinField = "id"
			}

			joinFieldType := resolveJoinFieldASLType(ir, link.TargetType, joinField)

			lf := Field{
				Name:       link.Name,
				Type:       link.TargetType,
				IsRequired: link.IsRequired,
				IsMulti:    link.IsMulti,
				OnTarget: OnTarget{
					Name: joinField,
					Type: joinFieldType,
				},
			}
			for _, c := range link.Constraints {
				lf.Constraints = append(lf.Constraints, Constraint{
					Name: c.Name,
					Args: c.Args,
				})
			}
			model.Fields = append(model.Fields, lf)
		}

		// Indexes → model-level indexes.
		for _, idx := range rt.Indexes {
			model.Indexes = append(model.Indexes, Index{
				Columns: append([]string(nil), idx.Columns...),
			})
		}

		// Type-level constraints → composite UNIQUE / PRIMARY KEY / length CHECK.
		for _, tc := range rt.Constraints {
			model.Constraints = append(model.Constraints, TypeConstraint{
				Expression: tc.Expression,
				Args:       append([]string(nil), tc.Args...),
				Columns:    append([]string(nil), tc.Columns...),
			})
		}

		models = append(models, model)
	}

	return models
}

// sqlTypeToASLType is the reverse of the SQL type map: SQL type → ASL type name.
func sqlTypeToASLType(sqlType string) string {
	m := map[string]string{
		"TEXT":             "str",
		"SMALLINT":         "int16",
		"INTEGER":          "int32",
		"BIGINT":           "int64",
		"REAL":             "float32",
		"DOUBLE PRECISION": "float64",
		"BOOLEAN":          "bool",
		"UUID":             "uuid",
		"TIMESTAMPTZ":      "datetime",
		"TIMESTAMP":        "datetime",
		"JSONB":            "json",
		"BYTEA":            "bytes",
		"NUMERIC":          "decimal",
		"DATE":             "date",
		"TIME":             "time",
	}
	if t, ok := m[sqlType]; ok {
		return t
	}
	return "str"
}

// resolveJoinFieldASLType returns the ASL type name for a referenced field
// in a target type. Defaults to "uuid" if not found.
func resolveJoinFieldASLType(ir *asl.SchemaIR, targetType, fieldName string) string {
	target, ok := ir.ObjectTypes[targetType]
	if !ok {
		return "uuid"
	}
	if prop, ok := target.Properties[fieldName]; ok {
		return sqlTypeToASLType(prop.SQLType)
	}
	return "uuid"
}
