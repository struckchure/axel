package asl

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// ConstraintFilterExpr captures the raw predicate of a constraint's `filter …`
// clause, which makes the constraint partial (lowered to a `CREATE UNIQUE INDEX
// … WHERE <predicate>`). Like PolicyExpr it keeps the embedded predicate
// decoupled from the ASL grammar, but — since the filter is not parenthesised —
// it consumes tokens up to the statement-terminating `;` at paren depth 0
// (leaving the `;` for the grammar). The predicate is native AQL, including the
// `.field` sugar, and is compiled to SQL later in the migration bridge.
//
// The receiver is invoked (via `@@`) positioned just after the `filter` keyword.
type ConstraintFilterExpr struct {
	Pos  lexer.Position
	toks []lexer.Token
}

// Parse implements participle.Parseable.
func (c *ConstraintFilterExpr) Parse(lex *lexer.PeekingLexer) error {
	c.Pos = lex.Peek().Pos
	depth := 0
	for {
		t := lex.Peek()
		if t.EOF() {
			return fmt.Errorf("unterminated constraint filter (missing ';')")
		}
		if t.Value == ";" && depth == 0 {
			if len(c.toks) == 0 {
				return fmt.Errorf("empty constraint filter")
			}
			return nil // leave ';' for the grammar
		}
		switch t.Value {
		case "(":
			depth++
		case ")":
			depth--
		}
		c.toks = append(c.toks, *lex.Next())
	}
}

// AQL reconstructs the raw predicate source from the captured tokens, inserting a
// space wherever two adjacent tokens weren't glued in the original. The result is
// native AQL, fed to aql.ParseExpr + the compiler for lowering — deferred to the
// migration bridge so this package stays free of any AQL/compiler dependency.
func (c *ConstraintFilterExpr) AQL() string {
	var sb strings.Builder
	for i := 0; i < len(c.toks); i++ {
		t := c.toks[i]
		if sb.Len() > 0 && i > 0 && !adjacent(c.toks[i-1], t) {
			sb.WriteByte(' ')
		}
		sb.WriteString(t.Value)
	}
	return strings.TrimSpace(sb.String())
}
