package lsp

import (
	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

var aqlStatementKeywords = []string{
	"multi", "select", "insert", "update", "delete", "filter",
	"order", "by", "limit", "offset", "set",
}

var aqlOperatorKeywords = []string{"and", "or", "in", "like", "ilike", "asc", "desc"}

var aslKeywords = []string{
	"scalar", "type", "model", "enum", "abstract", "extends", "extending",
	"required", "multi", "single", "property", "link", "constraint", "index",
	"on", "computed", "default",
}

// QueryCompletion returns context-aware completions for an AQL document at the
// given byte offset, resolved against the workspace schema (may be nil).
func QueryCompletion(text string, offset int, schema *asl.SchemaIR) []CompletionItem {
	wStart := wordStart(text, offset)
	prev, _ := prevSignificant(text, wStart)
	pw := prevWord(text, wStart)

	switch {
	case prev == '.':
		return fieldCompletions(text, schema)
	case prev == '<':
		return typeAnnotationCompletions(schema)
	case pw == "select" || pw == "insert" || pw == "update" || pw == "delete":
		items := typeNameCompletions(schema)
		if pw == "select" {
			items = append(items, CompletionItem{Label: "count", Detail: "aggregate", Kind: CompletionKindFunction})
		}
		return items
	case insideBraces(text, wStart):
		return fieldCompletions(text, schema)
	default:
		return append(keywordItems(aqlStatementKeywords), keywordItems(aqlOperatorKeywords)...)
	}
}

// SchemaCompletion returns completions for an ASL document: type names after a
// `:` annotation or `extending`, otherwise schema keywords.
func SchemaCompletion(text string, offset int, schema *asl.SchemaIR) []CompletionItem {
	wStart := wordStart(text, offset)
	prev, _ := prevSignificant(text, wStart)
	pw := prevWord(text, wStart)

	if prev == ':' || pw == "extending" || pw == "extends" {
		items := make([]CompletionItem, 0, len(builtinScalars))
		for _, b := range builtinScalars {
			items = append(items, CompletionItem{Label: b, Detail: "builtin", Kind: CompletionKindClass})
		}
		return append(items, typeNameCompletions(schema)...)
	}
	return keywordItems(aslKeywords)
}

// fieldCompletions lists the fields (+ `*` splat) of the query's type.
func fieldCompletions(text string, schema *asl.SchemaIR) []CompletionItem {
	items := []CompletionItem{{Label: "*", Detail: "all fields", Kind: CompletionKindKeyword}}
	if schema == nil {
		return items
	}
	rt := queryType(text, schema)
	if rt == nil {
		return items
	}
	for _, name := range sortedKeys(rt.Properties) {
		p := rt.Properties[name]
		items = append(items, CompletionItem{Label: p.Name, Detail: propType(p), Kind: CompletionKindField})
	}
	for _, name := range sortedKeys(rt.Links) {
		l := rt.Links[name]
		detail := l.TargetType
		if l.IsMulti {
			detail += "[]"
		}
		items = append(items, CompletionItem{Label: l.Name, Detail: detail, Kind: CompletionKindField})
	}
	return items
}

func typeNameCompletions(schema *asl.SchemaIR) []CompletionItem {
	if schema == nil {
		return nil
	}
	var items []CompletionItem
	for _, name := range sortedKeys(schema.ObjectTypes) {
		if schema.ObjectTypes[name].IsAbstract {
			continue
		}
		items = append(items, CompletionItem{Label: name, Detail: "type", Kind: CompletionKindClass})
	}
	return items
}

// typeAnnotationCompletions lists valid param-annotation types: builtins + enums
// + scalar aliases (not object types).
func typeAnnotationCompletions(schema *asl.SchemaIR) []CompletionItem {
	items := make([]CompletionItem, 0, len(builtinScalars))
	for _, b := range builtinScalars {
		items = append(items, CompletionItem{Label: b, Detail: "builtin", Kind: CompletionKindClass})
	}
	if schema != nil {
		for _, name := range sortedKeys(schema.EnumTypes) {
			items = append(items, CompletionItem{Label: name, Detail: "enum", Kind: CompletionKindEnum})
		}
		for _, name := range sortedKeys(schema.ScalarTypes) {
			items = append(items, CompletionItem{Label: name, Detail: "scalar", Kind: CompletionKindClass})
		}
	}
	return items
}

// queryType resolves the object type a query targets: parse first, else a
// lexical fallback that reads the identifier after the statement keyword.
func queryType(text string, schema *asl.SchemaIR) *asl.ResolvedType {
	if stmt, err := aql.ParseString(text); err == nil {
		if _, tn := stmtInfo(stmt); tn != "" {
			if rt, ok := schema.ObjectTypes[tn]; ok {
				return rt
			}
		}
	}
	if tn := lexicalQueryType(text); tn != "" {
		if rt, ok := schema.ObjectTypes[tn]; ok {
			return rt
		}
	}
	return nil
}

// lexicalQueryType finds the identifier following the first select/insert/
// update/delete keyword, tolerating an unparseable in-progress document.
func lexicalQueryType(text string) string {
	for _, kw := range []string{"select", "insert", "update", "delete"} {
		idx := indexWord(text, kw, 0)
		if idx < 0 {
			continue
		}
		i := idx + len(kw)
		for i < len(text) && !isWordByte(text[i]) {
			i++
		}
		j := i
		for j < len(text) && isWordByte(text[j]) {
			j++
		}
		if j > i {
			return text[i:j]
		}
	}
	return ""
}

func keywordItems(words []string) []CompletionItem {
	items := make([]CompletionItem, 0, len(words))
	for _, w := range words {
		items = append(items, CompletionItem{Label: w, Kind: CompletionKindKeyword})
	}
	return items
}

func wordStart(text string, offset int) int {
	if offset > len(text) {
		offset = len(text)
	}
	for offset > 0 && isWordByte(text[offset-1]) {
		offset--
	}
	return offset
}

// insideBraces reports whether pos sits inside an unclosed `{ … }` (a shape or
// an insert/update body), ignoring braces in comments and strings.
func insideBraces(text string, pos int) bool {
	depth := 0
	for i := 0; i < pos && i < len(text); i++ {
		switch text[i] {
		case '#':
			// skip to end of line
			for i < pos && i < len(text) && text[i] != '\n' {
				i++
			}
		case '\'':
			i++
			for i < pos && i < len(text) && text[i] != '\'' {
				i++
			}
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth > 0
}
