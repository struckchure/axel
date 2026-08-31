package aql

import (
	"fmt"
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
	"regexp"
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
	{Name: "Star", Pattern: `\*`},
	{Name: "Plus", Pattern: `\+`},
	{Name: "Minus", Pattern: `-`},
	{Name: "Slash", Pattern: `/`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "String", Pattern: `'[^']*'|"[^"]*"`},
	{Name: "Float", Pattern: `[0-9]+\.[0-9]+`},
	{Name: "Int", Pattern: `[0-9]+`},
	{Name: "Punct", Pattern: `[{};,:()\[\]$!|.?@]`},
})

var aqlParser = participle.MustBuild[Statement](
	participle.Lexer(aqlLexer),
	participle.Elide("Whitespace", "Comment"),
	participle.UseLookahead(4),
)

// exprParser parses a bare boolean expression (no surrounding statement). Used for
// contexts that embed an AQL predicate directly — e.g. RLS policy `using ( … )` /
// `with check ( … )` clauses.
var exprParser = participle.MustBuild[Expr](
	participle.Lexer(aqlLexer),
	participle.Elide("Whitespace", "Comment"),
	participle.UseLookahead(4),
)

// Parse parses a single AQL statement from src.
func Parse(src []byte) (*Statement, error) {
	stmt, err := aqlParser.ParseBytes("", src)
	return stmt, hintParseError(string(src), err)
}

// ParseString parses a single AQL statement from a string.
func ParseString(src string) (*Statement, error) {
	stmt, err := aqlParser.ParseString("", src)
	return stmt, hintParseError(src, err)
}

// prefixCastRe matches a C-style prefix cast — `<uuid>x`, `<str>.name` — which
// AQL does not have. The parser reports the failure at the *next* token, several
// characters past the real mistake, so the message alone rarely points at it.
var prefixCastRe = regexp.MustCompile(`<\s*([A-Za-z_][A-Za-z0-9_]*)\s*>\s*[A-Za-z_.$(]`)

// hintParseError appends a note naming the construct when the source contains a
// mistake the raw parser error describes poorly.
func hintParseError(src string, err error) error {
	if err == nil {
		return nil
	}
	if m := prefixCastRe.FindStringSubmatch(src); m != nil {
		return fmt.Errorf("%w\n  hint: casts in AQL are postfix — write `x<%s>`, not `<%s>x`", err, m[1], m[1])
	}
	return err
}

// ParseExpr parses a bare AQL boolean expression (the interior of a policy
// predicate). It has no trailing `;` and is not wrapped in a statement.
func ParseExpr(src string) (*Expr, error) {
	return exprParser.ParseString("", src)
}
