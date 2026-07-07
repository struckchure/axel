package lsp

import (
	"sort"
	"strings"

	"github.com/struckchure/axel/core/asl"
)

// builtinScalars are the AQL/ASL builtin value types (mirrors the Zed grammar).
var builtinScalars = []string{
	"str", "int16", "int32", "int64", "float32", "float64",
	"bool", "uuid", "datetime", "date", "time", "json", "bytes", "decimal",
}

// sqlToAQL maps a resolved SQL type back to its AQL/ASL name for display.
func sqlToAQL(sqlType string) string {
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

// propType returns the display type of a property (its enum name if enum-backed,
// else the AQL scalar).
func propType(p *asl.ResolvedProp) string {
	if p.EnumType != "" {
		return p.EnumType
	}
	return sqlToAQL(p.SQLType)
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

// fieldHover renders a one-line markdown summary for a field of rt, or "" if rt
// has no such field.
func fieldHover(rt *asl.ResolvedType, name string) string {
	if p, ok := rt.Properties[name]; ok {
		req := ""
		if p.IsRequired {
			req = "required "
		}
		return "```asl\n" + req + p.Name + ": " + propType(p) + "\n```"
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
