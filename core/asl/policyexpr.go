package asl

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// PolicyExpr captures the raw predicate of an RLS policy's `using ( … )` or
// `with check ( … )` clause. Like ReturnExpr it keeps the embedded SQL decoupled
// from the ASL grammar, consuming tokens up to the matching close paren (depth 0)
// without consuming it (the grammar's `')'` does that). The one piece of axel
// sugar is a leading `.field` path — rendered to the field's quoted column at
// lowering via SQL(); everything else passes through verbatim.
//
// The receiver is invoked (via `@@`) positioned just after the opening `(`.
type PolicyExpr struct {
	Pos  lexer.Position
	toks []lexer.Token
}

// Parse implements participle.Parseable.
func (p *PolicyExpr) Parse(lex *lexer.PeekingLexer) error {
	p.Pos = lex.Peek().Pos
	depth := 0
	for {
		t := lex.Peek()
		if t.EOF() {
			return fmt.Errorf("unterminated policy expression (missing ')')")
		}
		if t.Value == ")" && depth == 0 {
			if len(p.toks) == 0 {
				return fmt.Errorf("empty policy expression")
			}
			return nil // leave ')' for the grammar
		}
		switch t.Value {
		case "(":
			depth++
		case ")":
			depth--
		}
		p.toks = append(p.toks, *lex.Next())
	}
}

// AQL reconstructs the raw predicate source from the captured tokens, inserting a
// space wherever two adjacent tokens weren't glued in the original. The result is
// native AQL, fed to aql.ParseExpr + the compiler for lowering — deferred to the
// migration bridge so this package stays free of any AQL/compiler dependency
// (mirroring how inline trigger AQL bodies are stored raw and compiled later).
func (p *PolicyExpr) AQL() string {
	var sb strings.Builder
	for i := 0; i < len(p.toks); i++ {
		t := p.toks[i]
		if sb.Len() > 0 && i > 0 && !adjacent(p.toks[i-1], t) {
			sb.WriteByte(' ')
		}
		sb.WriteString(t.Value)
	}
	return strings.TrimSpace(sb.String())
}

func adjacent(a, b lexer.Token) bool {
	return b.Pos.Offset == a.Pos.Offset+len(a.Value)
}
