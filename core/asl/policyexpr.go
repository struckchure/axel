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

// SQL renders the predicate, rewriting leading `.field` path references to their
// quoted column via colOf. A `.` that follows an identifier / ')' / ']' / string
// is treated as member access (e.g. schema.func()) and left untouched.
func (p *PolicyExpr) SQL(colOf func(field string) (string, bool)) (string, error) {
	var sb strings.Builder
	for i := 0; i < len(p.toks); i++ {
		t := p.toks[i]
		space := sb.Len() > 0 && i > 0 && !adjacent(p.toks[i-1], t)

		if t.Value == "." && i+1 < len(p.toks) && startsIdent(p.toks[i+1].Value) && !memberAccess(p.toks, i) {
			field := p.toks[i+1].Value
			col, ok := colOf(field)
			if !ok {
				return "", fmt.Errorf("policy references unknown field .%s", field)
			}
			if space {
				sb.WriteByte(' ')
			}
			sb.WriteString(`"` + col + `"`)
			i++ // consume the ident
			continue
		}
		if space {
			sb.WriteByte(' ')
		}
		sb.WriteString(t.Value)
	}
	return strings.TrimSpace(sb.String()), nil
}

func adjacent(a, b lexer.Token) bool {
	return b.Pos.Offset == a.Pos.Offset+len(a.Value)
}

func startsIdent(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// memberAccess reports whether the '.' at index i is member access on a preceding
// operand (e.g. schema.func, a.b) rather than a leading axel path reference. It
// is member access only when the '.' is glued (no whitespace) to a preceding
// operand — an identifier/call-result/subscript/string. A '.' with space before
// it (`or .field`, `( .field`) is always a leading path reference.
func memberAccess(toks []lexer.Token, i int) bool {
	if i == 0 || !adjacent(toks[i-1], toks[i]) {
		return false
	}
	prev := toks[i-1].Value
	if prev == ")" || prev == "]" {
		return true
	}
	last := prev[len(prev)-1]
	return last == '_' || last == '\'' || (last >= 'a' && last <= 'z') ||
		(last >= 'A' && last <= 'Z') || (last >= '0' && last <= '9')
}
