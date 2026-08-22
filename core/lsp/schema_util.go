package lsp

import (
	"sort"
	"strings"

	"github.com/struckchure/axel/core/asl"
)

// builtinScalars are the AQL/ASL builtin value types (mirrors the Zed grammar).
var builtinScalars = []string{
	"str", "int16", "int32", "int64", "float32", "float64",
	"bool", "uuid", "datetime", "date", "time", "json", "jsonb", "bytes", "decimal",
}

// sqlToAQL maps a resolved SQL type back to its AQL/ASL name for display.
func sqlToAQL(sqlType string) string {
	if strings.HasSuffix(sqlType, "[]") {
		return sqlToAQL(strings.TrimSuffix(sqlType, "[]")) + "[]"
	}
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
		return strings.ToLower(sqlType)
	}
}

// propType returns the display type of a property (its enum name if enum-backed,
// its scalar type name if scalar-backed, else the AQL scalar).
func propType(p *asl.ResolvedProp) string {
	if p.EnumType != "" {
		return p.EnumType
	}
	if p.AQLType != "" {
		return p.AQLType
	}
	return sqlToAQL(p.SQLType)
}

// scalarHover renders a markdown summary of a scalar type definition.
func scalarHover(s *asl.ResolvedScalar) string {
	var b strings.Builder
	b.WriteString("```asl\n")
	if s.IsCustomSQL {
		b.WriteString("scalar type " + s.Name + " extends sql \"" + s.SQLType + "\"")
		if len(s.Fields) > 0 {
			b.WriteString(" as {\n")
			for _, name := range sortedKeys(s.Fields) {
				f := s.Fields[name]
				req := ""
				if f.IsRequired {
					req = "required "
				}
				multi := ""
				if f.IsMulti {
					multi = "multi "
				}
				expr := ""
				if f.ExprSQL != "" {
					expr = " := " + f.ExprSQL
				}
				b.WriteString("  " + req + multi + f.Name + ": " + f.AQLType + expr + ";\n")
			}
			b.WriteString("}\n")
		} else if s.IsMulti {
			b.WriteString(" as multi " + s.Base + ";\n")
		} else if s.Base != "" && s.Base != "str" {
			b.WriteString(" as " + s.Base + ";\n")
		} else {
			b.WriteString(";\n")
		}
	} else {
		base := s.Base
		if base == "" {
			base = sqlToAQL(s.SQLType)
		}
		hasBody := len(s.Fields) > 0 || len(s.Constraints) > 0 || s.Default != "" || len(s.Rewrites) > 0
		if !hasBody {
			b.WriteString("scalar type " + s.Name + " extends " + base + ";\n")
		} else {
			b.WriteString("scalar type " + s.Name + " extends " + base + " {\n")
			for _, c := range s.Constraints {
				args := ""
				if len(c.Args) > 0 {
					args = "(" + strings.Join(c.Args, ", ") + ")"
				}
				b.WriteString("  constraint " + c.Name + args + ";\n")
			}
			if s.Default != "" {
				b.WriteString("  default := " + s.Default + ";\n")
			}
			for _, r := range s.Rewrites {
				b.WriteString("  rewrite " + strings.Join(r.Events, ", ") + " := " + r.ValueSQL + ";\n")
			}
			for _, name := range sortedKeys(s.Fields) {
				f := s.Fields[name]
				req := ""
				if f.IsRequired {
					req = "required "
				}
				multi := ""
				if f.IsMulti {
					multi = "multi "
				}
				expr := ""
				if f.ExprSQL != "" {
					expr = " := " + f.ExprSQL
				}
				b.WriteString("  " + req + multi + f.Name + ": " + f.AQLType + expr + ";\n")
			}
			b.WriteString("}\n")
		}
	}
	b.WriteString("```")
	return b.String()
}

// typeHover renders a markdown summary of an object type: its fields and links.
func typeHover(rt *asl.ResolvedType) string {
	var b strings.Builder
	kind := "type"
	if rt.IsAbstract {
		kind = "abstract type"
	}
	b.WriteString("```asl\n")
	b.WriteString(kind + " " + rt.Name + " {\n")
	for _, name := range sortedKeys(rt.Properties) {
		p := rt.Properties[name]
		req := ""
		if p.IsRequired {
			req = "required "
		}
		b.WriteString("  " + req + p.Name + ": " + propType(p) + ";\n")
	}
	for _, name := range sortedKeys(rt.Links) {
		l := rt.Links[name]
		mod := "link "
		if l.IsMulti {
			mod = "multi link "
		}
		b.WriteString("  " + mod + l.Name + ": " + l.TargetType + ";\n")
	}
	b.WriteString("}\n```")
	return b.String()
}

// functionHover renders a markdown summary for a declared PostgreSQL function.
func functionHover(fn *asl.ResolvedFunction) string {
	var b strings.Builder
	b.WriteString("```asl\n")
	if fn.Language != "" && fn.Language != "plpgsql" {
		b.WriteString("@language " + fn.Language + "\n")
	}
	if fn.Volatility != "" {
		b.WriteString("@" + fn.Volatility + "\n")
	}
	if fn.Strict {
		b.WriteString("@strict\n")
	}
	if fn.Parallel != "" {
		b.WriteString("@parallel " + fn.Parallel + "\n")
	}
	if fn.Security != "" {
		b.WriteString("@security " + fn.Security + "\n")
	}
	params := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		params[i] = p.Name + ": " + sqlToAQL(p.SQLType)
	}
	ret := sqlToAQL(fn.Returns)
	b.WriteString("function " + fn.Name + "(" + strings.Join(params, ", ") + ") -> " + ret + "\n```")
	return b.String()
}

// fieldHover renders a one-line markdown summary for a field of rt, or "" if rt
// has no such field.
func fieldHover(rt *asl.ResolvedType, name string, schema *asl.SchemaIR) string {
	if p, ok := rt.Properties[name]; ok {
		req := ""
		if p.IsRequired {
			req = "required "
		}
		header := "```asl\n" + req + p.Name + ": " + propType(p) + "\n```"
		if schema != nil && p.AQLType != "" {
			if s, ok := schema.ScalarTypes[p.AQLType]; ok && len(s.Fields) > 0 {
				header += "\n\n" + scalarHover(s)
			}
		}
		return header
	}
	if l, ok := rt.Links[name]; ok {
		mod := "link"
		if l.IsMulti {
			mod = "multi link"
		}
		return "```asl\n" + mod + " " + l.Name + ": " + l.TargetType + "\n```"
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
