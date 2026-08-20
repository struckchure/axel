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
		if s, ok := schema.ScalarTypes[word]; ok {
			return &Hover{Contents: scalarHover(s), Range: rng}
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
// document, resolved against the workspace schema: a directive summary, type summary, or a
// `field: type` line for a field of the query's type.
func QueryHover(text string, offset int, schema *asl.SchemaIR) *Hover {
	word, start, end := wordAt(text, offset)
	if word == "" {
		return nil
	}
	rng := Range{Start: OffsetToPosition(text, start), End: OffsetToPosition(text, end)}

	switch word {
	case "name":
		return &Hover{Contents: "```aql\n@name <Identifier>\n```\nOverrides the generated query function name.", Range: rng}
	case "request":
		return &Hover{Contents: "```aql\n@request <TypeName>\n```\nOverrides the generated request parameter struct / interface name.", Range: rng}
	case "response":
		return &Hover{Contents: "```aql\n@response <TypeName>\n```\nOverrides the generated response row struct / interface name.", Range: rng}
	case "rel_load_strategy":
		return &Hover{Contents: "```aql\n@rel_load_strategy <query | join>\n```\nConfigures how relation shapes and nested subqueries are compiled into SQL:\n\n- `query` (default): Emits correlated `json_agg` / `row_to_json` subqueries in the SELECT clause.\n- `join`: Emits `LEFT JOIN LATERAL` clauses in the FROM clause.", Range: rng}
	case "join":
		return &Hover{Contents: "**Relation Load Strategy: `join`**\n\nCompiles relation links and subqueries using `LEFT JOIN LATERAL` in the SQL FROM clause.", Range: rng}
	case "query":
		return &Hover{Contents: "**Relation Load Strategy: `query`** (default)\n\nCompiles relation links and subqueries using correlated scalar subqueries (`json_agg` / `row_to_json`) in the SELECT column list.", Range: rng}
	}

	if schema == nil {
		return nil
	}

	if rt, ok := schema.ObjectTypes[word]; ok {
		return &Hover{Contents: typeHover(rt), Range: rng}
	}
	if s, ok := schema.ScalarTypes[word]; ok {
		return &Hover{Contents: scalarHover(s), Range: rng}
	}
	if e, ok := schema.EnumTypes[word]; ok {
		return &Hover{Contents: "```asl\nenum " + e.Name + " { " + join(e.Values) + " }\n```", Range: rng}
	}

	// Field of the query's type (or nested typed JSON scalar field)?
	if stmt, err := aql.ParseString(text); err == nil {
		if _, tn := stmtInfo(stmt); tn != "" {
			if rt, ok := schema.ObjectTypes[tn]; ok {
				if md := fieldHover(rt, word, schema); md != "" {
					return &Hover{Contents: md, Range: rng}
				}
				// Check if word is a subfield of a typed JSON scalar: e.g. in `.coord.lat`, word is "lat"
				if start > 0 && text[start-1] == '.' {
					pw := prevWord(text, start-1)
					if prop, ok := rt.Properties[pw]; ok && prop.AQLType != "" {
						if scalar, ok := schema.ScalarTypes[prop.AQLType]; ok && scalar.Fields != nil {
							if sf, ok := scalar.Fields[word]; ok {
								req := ""
								if sf.IsRequired {
									req = "required "
								}
								multi := ""
								if sf.IsMulti {
									multi = "multi "
								}
								return &Hover{
									Contents: "```asl\n" + req + multi + sf.Name + ": " + sf.AQLType + "\n```\n\n(field of `scalar type " + scalar.Name + "`)\n\n" + scalarHover(scalar),
									Range:    rng,
								}
							}
						}
					}
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
