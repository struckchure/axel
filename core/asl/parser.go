package asl

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// aslLexer defines tokens for the Axel Schema Language.
var aslLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `#[^\n]*`},
	{Name: "Whitespace", Pattern: `\s+`},
	// Multi-char tokens must come before single-char Punct
	{Name: "Assign", Pattern: `:=`},
	{Name: "Coalesce", Pattern: `\?\?`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "String", Pattern: `'[^']*'`},
	{Name: "Int", Pattern: `[0-9]+`},
	{Name: "Punct", Pattern: `[{};,:()\[\]@!<>=|.?]`},
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
