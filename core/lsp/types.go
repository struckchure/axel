// Package lsp provides editor-agnostic language analysis for Axel's ASL schema
// and AQL query languages: diagnostics, document symbols, hover, definition, and
// completion. It has no dependency on any LSP transport library — the server in
// cmd/lsp.go maps these types onto the LSP protocol. Numeric enum values mirror
// the LSP spec so that mapping is a straight cast.
package lsp

// Position is a zero-based line and UTF-16 character offset (LSP semantics).
type Position struct {
	Line int
	Char int
}

// Range is a half-open span between two Positions.
type Range struct {
	Start Position
	End   Position
}

// Severity mirrors LSP DiagnosticSeverity.
type Severity int

const (
	SeverityError   Severity = 1
	SeverityWarning Severity = 2
	SeverityInfo    Severity = 3
	SeverityHint    Severity = 4
)

// Diagnostic is a single problem reported for a document.
type Diagnostic struct {
	Range    Range
	Severity Severity
	Message  string
}

// SymbolKind mirrors LSP SymbolKind.
type SymbolKind int

const (
	SymbolKindFunction SymbolKind = 12
	SymbolKindEnum     SymbolKind = 10
	SymbolKindProperty SymbolKind = 7
	SymbolKindField    SymbolKind = 8
	SymbolKindStruct   SymbolKind = 23
	SymbolKindVariable SymbolKind = 13
	SymbolKindEnumMbr  SymbolKind = 22
)

// Symbol is a document symbol (optionally nested), used for the outline.
type Symbol struct {
	Name      string
	Detail    string
	Kind      SymbolKind
	Range     Range // full extent of the declaration
	Selection Range // the name token
	Children  []Symbol
}

// Location points at a range within a document URI (for go-to-definition).
type Location struct {
	URI   string
	Range Range
}

// Hover is markdown content shown for the symbol under the cursor.
type Hover struct {
	Contents string
	Range    Range
}

// CompletionKind mirrors LSP CompletionItemKind.
type CompletionKind int

const (
	CompletionKindFunction CompletionKind = 3
	CompletionKindField    CompletionKind = 5
	CompletionKindVariable CompletionKind = 6
	CompletionKindClass    CompletionKind = 7
	CompletionKindEnum     CompletionKind = 13
	CompletionKindKeyword  CompletionKind = 14
)

// CompletionItem is one suggestion.
type CompletionItem struct {
	Label  string
	Detail string
	Kind   CompletionKind
}
