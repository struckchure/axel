package aql

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// aqlLexer defines tokens for the Axel Query Language.
var aqlLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `#[^\n]*`},
	{Name: "Whitespace", Pattern: `\s+`},
	// Multi-char operators must come before their single-char prefixes.
	{Name: "NEq", Pattern: `!=`},
	{Name: "LtEq", Pattern: `<=`},
	{Name: "GtEq", Pattern: `>=`},
	{Name: "Assign", Pattern: `:=`},
	{Name: "Coalesce", Pattern: `\?\?`},
	// Single-char operators need their own rules to appear in participle's
	// symbol table; otherwise lookahead matching for optional groups fails.
	{Name: "Eq", Pattern: `=`},
	{Name: "Lt", Pattern: `<`},
	{Name: "Gt", Pattern: `>`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "String", Pattern: `'[^']*'`},
	{Name: "Float", Pattern: `[0-9]+\.[0-9]+`},
	{Name: "Int", Pattern: `[0-9]+`},
	{Name: "Punct", Pattern: `[{};,:()\[\]$!|.?@]`},
})

var aqlParser = participle.MustBuild[Statement](
	participle.Lexer(aqlLexer),
	participle.Elide("Whitespace", "Comment"),
	participle.UseLookahead(4),
)

// Parse parses a single AQL statement from src.
func Parse(src []byte) (*Statement, error) {
	return aqlParser.ParseBytes("", src)
}

// ParseString parses a single AQL statement from a string.
func ParseString(src string) (*Statement, error) {
	return aqlParser.ParseString("", src)
}
