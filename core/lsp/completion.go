package lsp

import (
	"strings"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

var aqlStatementKeywords = []string{
	"with", "multi", "select", "insert", "update", "delete", "filter",
	"order", "by", "limit", "offset", "set",
	"unless", "conflict", "on", "else",
}

var aqlOperatorKeywords = []string{"and", "or", "in", "like", "ilike", "asc", "desc"}

var aslKeywords = []string{
	"scalar", "type", "model", "enum", "abstract", "extends", "extending",
	"required", "multi", "single", "property", "link", "constraint", "index",
	"on", "filter", "computed", "default",
	"rewrite", "trigger", "function", "before", "after", "do", "execute",
	"when", "for", "each", "row", "statement", "language", "body",
	// Row-level security policies
	"policy", "using", "with", "check", "select", "insert", "update", "delete", "all", "to",
	// Functions & extensions
	"use", "extension", "return",
	// Function attribute directives (used as @name)
	"immutable", "stable", "volatile", "strict", "leakproof",
	"parallel", "safe", "unsafe", "restricted", "security", "definer",
	"invoker", "cost",
}

// QueryCompletion returns context-aware completions for an AQL document at the
// given byte offset, resolved against the workspace schema (may be nil).
func QueryCompletion(text string, offset int, schema *asl.SchemaIR) []CompletionItem {
	wStart := wordStart(text, offset)
	prev, prevIdx := prevSignificant(text, wStart)
	pw := prevWord(text, wStart)

	switch {
	case prev == '.':
		// `EnumName.` completes enum members; otherwise the query type's fields.
		if items := enumMemberCompletions(text, prevIdx, schema); items != nil {
			return items
		}
		return fieldCompletions(text, schema)
	case prev == '=':
		// Right-hand side of a comparison (`=`, `!=`, `<=`, `>=`) whose left side
		// is an enum-typed field → suggest that enum's members.
		if items := enumComparisonCompletions(text, prevIdx, schema); items != nil {
			return items
		}
		return append(keywordItems(aqlStatementKeywords), keywordItems(aqlOperatorKeywords)...)
	case prev == '<':
		return typeAnnotationCompletions(schema)
	case pw == "select" || pw == "insert" || pw == "update" || pw == "delete":
		items := typeNameCompletions(schema)
		if pw == "select" {
			items = append(items, CompletionItem{Label: "count", Detail: "aggregate", Kind: CompletionKindFunction})
		}
		return items
	// Operand position: a filter condition, either arm of a boolean chain, or an
	// `order by` term. All take a `.field` path. Checked before insideBraces — a
	// filter can sit inside a shape, in a computed field's sub-select.
	case pw == "filter" || pw == "and" || pw == "or" || pw == "by":
		return pathCompletions(text, schema)
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
	prev, prevIdx := prevSignificant(text, wStart)
	pw := prevWord(text, wStart)

	// `default := Enum.` (and any `EnumName.` such as a constraint filter RHS)
	// completes that enum's members.
	if prev == '.' {
		return enumMemberCompletions(text, prevIdx, schema)
	}
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
	return append([]CompletionItem{{Label: "*", Detail: "all fields", Kind: CompletionKindKeyword}},
		typeFieldCompletions(text, schema, "")...)
}

// pathCompletions lists the query type's fields as `.field` paths — the form a
// filter or `order by` operand takes.
func pathCompletions(text string, schema *asl.SchemaIR) []CompletionItem {
	return typeFieldCompletions(text, schema, ".")
}

// typeFieldCompletions lists the query type's properties and links, each label
// carrying the given prefix.
func typeFieldCompletions(text string, schema *asl.SchemaIR, prefix string) []CompletionItem {
	if schema == nil {
		return nil
	}
	rt := queryType(text, schema)
	if rt == nil {
		return nil
	}
	var items []CompletionItem
	for _, name := range sortedKeys(rt.Properties) {
		p := rt.Properties[name]
		items = append(items, CompletionItem{Label: prefix + p.Name, Detail: propType(p), Kind: CompletionKindField})
	}
	for _, name := range sortedKeys(rt.Links) {
		l := rt.Links[name]
		detail := l.TargetType
		if l.IsMulti {
			detail += "[]"
		}
		items = append(items, CompletionItem{Label: prefix + l.Name, Detail: detail, Kind: CompletionKindField})
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

// enumMemberCompletions returns the members of the enum named immediately before
// the dot at dotIdx (e.g. the `QueueStatus` in `QueueStatus.`), or nil if that
// identifier is not a known enum type.
func enumMemberCompletions(text string, dotIdx int, schema *asl.SchemaIR) []CompletionItem {
	if schema == nil || dotIdx < 0 {
		return nil
	}
	enum, ok := schema.EnumTypes[prevWord(text, dotIdx)]
	if !ok {
		return nil
	}
	return enumValueItems(enum, "")
}

// enumComparisonCompletions handles the RHS of a comparison operator ending at
// opIdx: if the compared field is enum-typed, it suggests the enum's members as
// qualified `Enum.Value` labels. Returns nil when the field isn't enum-typed.
func enumComparisonCompletions(text string, opIdx int, schema *asl.SchemaIR) []CompletionItem {
	if schema == nil || opIdx < 0 {
		return nil
	}
	// Step left over the operator run (=, !=, <=, >=) to the field being compared.
	i := opIdx
	for i >= 0 && strings.IndexByte("=!<>", text[i]) >= 0 {
		i--
	}
	field := prevWord(text, i+1)
	if field == "" {
		return nil
	}
	rt := queryType(text, schema)
	if rt == nil {
		return nil
	}
	p, ok := rt.Properties[field]
	if !ok || p.EnumType == "" {
		return nil
	}
	enum, ok := schema.EnumTypes[p.EnumType]
	if !ok {
		return nil
	}
	return enumValueItems(enum, p.EnumType+".")
}

// enumValueItems renders an enum's members as completion items, each label
// carrying the given prefix (e.g. "Status." for a qualified suggestion).
func enumValueItems(enum *asl.ResolvedEnum, prefix string) []CompletionItem {
	items := make([]CompletionItem, 0, len(enum.Values))
	for _, v := range enum.Values {
		items = append(items, CompletionItem{Label: prefix + v, Detail: enum.Name, Kind: CompletionKindEnumMember})
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
