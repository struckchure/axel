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
	// AQLString is a backtick-delimited inline AQL query (` aql`select …` `). Like
	// DollarString it is one opaque token: the AQL inside is never tokenized by the
	// ASL lexer, and is handed to the AQL parser at lowering time. See ReturnExpr.
	{Name: "AQLString", Pattern: "`[^`]*`"},
	// Multi-char tokens must come before single-char Punct
	{Name: "Assign", Pattern: `:=`},
	{Name: "Arrow", Pattern: `->`},
	{Name: "Coalesce", Pattern: `\?\?`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "String", Pattern: `'[^']*'|"[^"]*"`},
	{Name: "Float", Pattern: `[0-9]+\.[0-9]+`},
	{Name: "Int", Pattern: `[0-9]+`},
	// `$` (AQL param prefix) and `*` (AQL splat) are here so an embedded AQL body
	// (see AQLBlock) tokenizes without error; the arithmetic/operator chars
	// (`+ - / % ^ & ~`) let a raw `return <sql-expr>;` body (see ReturnExpr)
	// tokenize. No ASL rule references any of these directly.
	{Name: "Punct", Pattern: `[{};,:()\[\]@!<>=|.?$*+/%^&~-]`},
})

var aslParser = participle.MustBuild[SourceFile](
	participle.Lexer(aslLexer),
	participle.Elide("Whitespace", "Comment"),
	participle.UseLookahead(3),
)

// Parse parses a .asl schema file and returns the AST root. Positions carry no
// filename; use ParseNamed when the source came from a known path so errors can
// name the file it came from.
func Parse(src []byte) (*SourceFile, error) {
	return ParseNamed("", src)
}

// ParseNamed parses a .asl schema file, attributing every position in the
// resulting AST to filename. This is what lets a schema split across several
// files report "type %q declared more than once" with both file locations.
func ParseNamed(filename string, src []byte) (*SourceFile, error) {
	return aslParser.ParseBytes(filename, src)
}
