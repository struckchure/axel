package lsp

import (
	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

// SchemaHover returns hover text for the identifier under offset in an ASL
// document: a summary of the referenced type/enum, or a builtin note.
func SchemaHover(text string, offset int, schema *asl.SchemaIR) *Hover {
	word, start, end := wordAt(text, offset)
	if word == "" {
		return nil
	}
	rng := Range{Start: OffsetToPosition(text, start), End: OffsetToPosition(text, end)}
	if schema != nil {
		if rt, ok := schema.ObjectTypes[word]; ok {
			return &Hover{Contents: typeHover(rt), Range: rng}
		}
		if e, ok := schema.EnumTypes[word]; ok {
			return &Hover{Contents: "```asl\nenum " + e.Name + " { " + join(e.Values) + " }\n```", Range: rng}
		}
	}
	if isBuiltin(word) {
		return &Hover{Contents: "```asl\nbuiltin scalar " + word + "\n```", Range: rng}
	}
	return nil
}

// QueryHover returns hover text for the identifier under offset in an AQL
// document, resolved against the workspace schema: a type summary, or a
// `field: type` line for a field of the query's type.
func QueryHover(text string, offset int, schema *asl.SchemaIR) *Hover {
	if schema == nil {
		return nil
	}
	word, start, end := wordAt(text, offset)
	if word == "" {
		return nil
	}
	rng := Range{Start: OffsetToPosition(text, start), End: OffsetToPosition(text, end)}

	if rt, ok := schema.ObjectTypes[word]; ok {
		return &Hover{Contents: typeHover(rt), Range: rng}
	}
	if e, ok := schema.EnumTypes[word]; ok {
		return &Hover{Contents: "```asl\nenum " + e.Name + " { " + join(e.Values) + " }\n```", Range: rng}
	}

	// Field of the query's type?
	if stmt, err := aql.ParseString(text); err == nil {
		if _, tn := stmtInfo(stmt); tn != "" {
			if rt, ok := schema.ObjectTypes[tn]; ok {
				if md := fieldHover(rt, word); md != "" {
					return &Hover{Contents: md, Range: rng}
				}
			}
		}
	}
	return nil
}

func isBuiltin(word string) bool {
	for _, b := range builtinScalars {
		if b == word {
			return true
		}
	}
	return false
}

func join(vals []string) string {
	out := ""
	for i, v := range vals {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}
