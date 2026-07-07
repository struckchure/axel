package lsp

import (
	"strings"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

// SchemaSymbols returns the document outline for an ASL schema: types (with
// their fields as children), enums, and scalar aliases.
func SchemaSymbols(text string) []Symbol {
	sf, err := asl.Parse([]byte(text))
	if err != nil || sf == nil {
		return nil
	}
	var syms []Symbol
	for _, d := range sf.Definitions {
		switch {
		case d.TypeDef != nil:
			td := d.TypeDef
			s := Symbol{
				Name:      td.Name,
				Detail:    typeDetail(td),
				Kind:      SymbolKindStruct,
				Range:     nodeRange(text, td.Pos, td.EndPos),
				Selection: nameSelection(text, td.Pos, td.Name),
			}
			for _, m := range td.Members {
				if m.Field == nil {
					continue
				}
				f := m.Field
				kind := SymbolKindProperty
				if f.LinkKeyword {
					kind = SymbolKindField
				}
				s.Children = append(s.Children, Symbol{
					Name:      f.Name,
					Detail:    fieldDetail(f),
					Kind:      kind,
					Range:     nodeRange(text, f.Pos, f.EndPos),
					Selection: nameSelection(text, f.Pos, f.Name),
				})
			}
			syms = append(syms, s)
		case d.EnumType != nil:
			et := d.EnumType
			syms = append(syms, Symbol{
				Name:      et.Name,
				Detail:    "enum",
				Kind:      SymbolKindEnum,
				Range:     nodeRange(text, et.Pos, et.EndPos),
				Selection: nameSelection(text, et.Pos, et.Name),
			})
		case d.ScalarType != nil:
			st := d.ScalarType
			syms = append(syms, Symbol{
				Name:      st.Name,
				Detail:    "scalar extending " + st.Extends,
				Kind:      SymbolKindStruct,
				Range:     nodeRange(text, st.Pos, st.EndPos),
				Selection: nameSelection(text, st.Pos, st.Name),
			})
		}
	}
	return syms
}

// QuerySymbols returns the outline for an AQL query: a root for the statement,
// with its directives and parameters as children.
func QuerySymbols(text string) []Symbol {
	stmt, err := aql.ParseString(text)
	if err != nil || stmt == nil {
		return nil
	}
	op, typeName := stmtInfo(stmt)
	label := op
	if typeName != "" {
		label += " " + typeName
	}
	root := Symbol{
		Name:      label,
		Kind:      SymbolKindStruct,
		Range:     nodeRange(text, stmt.Pos, stmt.EndPos),
		Selection: nodeRange(text, stmt.Pos, stmt.EndPos),
	}
	for _, d := range stmt.Directives {
		root.Children = append(root.Children, Symbol{
			Name:      "@" + d.Name + " " + d.Value,
			Kind:      SymbolKindProperty,
			Range:     nodeRange(text, d.Pos, d.EndPos),
			Selection: nodeRange(text, d.Pos, d.EndPos),
		})
	}
	seen := map[string]bool{}
	for _, p := range scanParams(text) {
		if seen[p.name] {
			continue
		}
		seen[p.name] = true
		root.Children = append(root.Children, Symbol{
			Name:      "$" + p.name,
			Kind:      SymbolKindVariable,
			Range:     p.rng,
			Selection: p.rng,
		})
	}
	return []Symbol{root}
}

func typeDetail(td *asl.TypeDef) string {
	switch {
	case td.Abstract:
		return "abstract type"
	default:
		return "type"
	}
}

func fieldDetail(f *asl.FieldDecl) string {
	var b strings.Builder
	if f.Multi {
		b.WriteString("multi ")
	}
	if f.LinkKeyword {
		b.WriteString("link ")
	}
	if f.TypeSpec != nil && f.TypeSpec.PropType != nil {
		b.WriteString(*f.TypeSpec.PropType)
	}
	return strings.TrimSpace(b.String())
}

// stmtInfo returns the operation name and (best-effort) target type of a query.
func stmtInfo(stmt *aql.Statement) (op, typeName string) {
	switch {
	case stmt.Select != nil:
		op = "select"
		if b := stmt.Select.Body; b != nil {
			if b.AggFunc != nil {
				typeName = b.AggFunc.TypeName
			} else {
				typeName = b.TypeName
			}
		}
	case stmt.Insert != nil:
		op, typeName = "insert", stmt.Insert.TypeName
	case stmt.Update != nil:
		op, typeName = "update", stmt.Update.TypeName
	case stmt.Delete != nil:
		op, typeName = "delete", stmt.Delete.TypeName
	}
	return op, typeName
}

type paramMatch struct {
	name string
	rng  Range
}

// scanParams finds `$name` parameter tokens in text (lexically, so it works even
// when the document doesn't fully parse).
func scanParams(text string) []paramMatch {
	var out []paramMatch
	for i := 0; i < len(text); i++ {
		if text[i] != '$' || commentStart(text, i) >= 0 {
			continue
		}
		j := i + 1
		for j < len(text) && isWordByte(text[j]) {
			j++
		}
		if j == i+1 {
			continue
		}
		out = append(out, paramMatch{
			name: text[i+1 : j],
			rng:  Range{Start: OffsetToPosition(text, i), End: OffsetToPosition(text, j)},
		})
	}
	return out
}
