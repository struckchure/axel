package asl

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// ReturnExpr captures the raw Postgres expression of a function's
// `return <sql-expr>;` body. Like AQLBlock it keeps the embedded SQL fully
// decoupled from the ASL grammar: it consumes every token up to the terminating
// top-level `;` (paren depth 0) and stores the reconstructed text, which the
// lowerer wraps in a plpgsql `BEGIN RETURN … ; END;` (or a `SELECT …;` for a
// `sql`-language function). The expression passes through to Postgres verbatim.
//
// The receiver is invoked (via a `@@` reference) positioned just after `return`.
type ReturnExpr struct {
	Pos lexer.Position
	Raw string // reconstructed source of the expression (terminating ';' excluded)
}

// Parse implements participle.Parseable. It consumes tokens through the matching
// top-level `;`, tracking paren depth so a `;` inside a parenthesized sub-list
// (there shouldn't be one in an expression, but be safe) doesn't terminate early.
// String and dollar-quoted tokens are opaque, so parens/semicolons inside them
// never affect the counts.
func (r *ReturnExpr) Parse(lex *lexer.PeekingLexer) error {
	r.Pos = lex.Peek().Pos

	depth := 0
	var toks []lexer.Token
	for {
		t := lex.Peek()
		if t.EOF() {
			return fmt.Errorf("unterminated return expression (missing ';')")
		}
		switch t.Value {
		case "(":
			depth++
		case ")":
			depth--
		case ";":
			if depth == 0 {
				lex.Next() // consume the terminating ';'
				r.Raw = reconstructTokens(toks)
				if strings.TrimSpace(r.Raw) == "" {
					return fmt.Errorf("empty return expression")
				}
				return nil
			}
		}
		toks = append(toks, *lex.Next())
	}
}
