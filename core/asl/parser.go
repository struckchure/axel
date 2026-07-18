package asl

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// aslLexer defines tokens for the Axel Schema Language.
var aslLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `#[^\n]*`},
	{Name: "Whitespace", Pattern: `\s+`},
	// DollarString is Postgres dollar-quoting ($$ … $$) used for raw SQL trigger
	// and function bodies. It must precede Ident/Punct so the whole span is one
	// opaque token (its interior is never tokenized).
	{Name: "DollarString", Pattern: `\$\$[\s\S]*?\$\$`},
	// Multi-char tokens must come before single-char Punct
	{Name: "Assign", Pattern: `:=`},
	{Name: "Arrow", Pattern: `->`},
	{Name: "Coalesce", Pattern: `\?\?`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "String", Pattern: `'[^']*'`},
	{Name: "Int", Pattern: `[0-9]+`},
	// `$` (AQL param prefix) and `*` (AQL splat) are here so an embedded AQL body
	// (see AQLBlock) tokenizes without error; no ASL rule references them directly.
	{Name: "Punct", Pattern: `[{};,:()\[\]@!<>=|.?$*]`},
})

var aslParser = participle.MustBuild[SourceFile](
	participle.Lexer(aslLexer),
	participle.Elide("Whitespace", "Comment"),
	participle.UseLookahead(3),
)

// Parse parses a .asl schema file and returns the AST root.
func Parse(src []byte) (*SourceFile, error) {
	return aslParser.ParseBytes("", src)
}
