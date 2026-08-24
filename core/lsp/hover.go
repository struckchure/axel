package lsp

import (
	"strings"

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
		if fn, ok := schema.Functions[word]; ok {
			return &Hover{Contents: functionHover(fn), Range: rng}
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
	// 1. Variable / Parameter hover ($param reference)
	if pName, pStart, pEnd, isParam := paramNameAt(text, offset); isParam {
		pRng := Range{Start: OffsetToPosition(text, pStart), End: OffsetToPosition(text, pEnd)}
		if h := paramHover(text, pName, pRng, schema); h != nil {
			return h
		}
	}

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

	// 2. With-binding or var param by name (when hovering on identifier in var or with block)
	if stmt, err := aql.ParseString(text); err == nil && stmt != nil {
		if h := varOrWithHover(stmt, word, rng, schema); h != nil {
			return h
		}
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
	if fn, ok := schema.Functions[word]; ok {
		return &Hover{Contents: functionHover(fn), Range: rng}
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
								expr := ""
								if sf.ExprSQL != "" {
									expr = " := " + sf.ExprSQL
								}
								return &Hover{
									Contents: "```asl\n" + req + multi + sf.Name + ": " + sf.AQLType + expr + "\n```\n\n(field of `scalar type " + scalar.Name + "`)\n\n" + scalarHover(scalar),
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

func paramHover(text, paramName string, rng Range, schema *asl.SchemaIR) *Hover {
	stmt, _ := aql.ParseString(text)
	if stmt != nil {
		if stmt.For != nil {
			iter := strings.TrimPrefix(stmt.For.Iterator, "$")
			if iter == paramName {
				return &Hover{
					Contents: "```aql\nfor $" + iter + " in ...\n```\nLoop iterator variable",
					Range:    rng,
				}
			}
		}
		for _, v := range stmt.Vars {
			for _, p := range v.Params {
				if p.Name == paramName {
					return formatParamHover(p, rng, schema)
				}
			}
		}
	}
	return &Hover{
		Contents: "```aql\nvar $" + paramName + "\n```\nQuery parameter",
		Range:    rng,
	}
}

func formatParamHover(p *aql.VarParam, rng Range, schema *asl.SchemaIR) *Hover {
	multiPrefix := ""
	if p.Multi {
		multiPrefix = "multi "
	}
	typeAnnot := ""
	typeKey := p.Type
	if p.ColonType != "" {
		typeAnnot = ": " + p.ColonType
		if typeKey == "" {
			typeKey = p.ColonType
		}
	} else if p.Type != "" {
		typeAnnot = "<" + p.Type + ">"
	}
	opt := ""
	if p.Optional {
		opt = "?"
	}
	content := "```aql\nvar " + multiPrefix + "$" + p.Name + typeAnnot + opt + "\n```\nQuery parameter"
	if schema != nil && typeKey != "" {
		if s, ok := schema.ScalarTypes[typeKey]; ok {
			content += "\n\n" + scalarHover(s)
		} else if e, ok := schema.EnumTypes[typeKey]; ok {
			content += "\n\n```asl\nenum " + e.Name + " { " + join(e.Values) + " }\n```"
		} else if rt, ok := schema.ObjectTypes[typeKey]; ok {
			content += "\n\n" + typeHover(rt)
		}
	}
	return &Hover{
		Contents: content,
		Range:    rng,
	}
}

func varOrWithHover(stmt *aql.Statement, word string, rng Range, schema *asl.SchemaIR) *Hover {
	if stmt.For != nil {
		iter := strings.TrimPrefix(stmt.For.Iterator, "$")
		if iter == word {
			return &Hover{
				Contents: "```aql\nfor $" + iter + " in ...\n```\nLoop iterator variable",
				Range:    rng,
			}
		}
	}
	if stmt.With != nil {
		for _, b := range stmt.With.Bindings {
			if b.Name == word {
				return &Hover{
					Contents: "```aql\nwith " + b.Name + " := ...\n```\nSubquery CTE binding",
					Range:    rng,
				}
			}
		}
	}
	for _, v := range stmt.Vars {
		for _, p := range v.Params {
			if p.Name == word {
				return formatParamHover(p, rng, schema)
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
