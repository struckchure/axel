package asl

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// AQLBlock captures a parenthesized AQL body embedded in a `.asl` file — the
// `do ( … )` of a trigger or the `body := ( … )` of a function. The ASL grammar
// does not parse the AQL itself; this custom Parseable consumes the balanced
// `( … )` span and stores the raw text, which the resolver later hands to
// aql.ParseString. This keeps the two grammars fully decoupled.
//
// The receiver is invoked (via a `@@` reference) positioned at the opening `(`.
type AQLBlock struct {
	Pos lexer.Position
	Raw string // reconstructed source between the outermost parens (exclusive)
}

// Parse implements participle.Parseable. It expects the lexer positioned at `(`
// and consumes through the matching `)`, tracking paren depth. Dollar-quoted and
// string tokens are opaque, so parens inside them never affect the depth count.
func (b *AQLBlock) Parse(lex *lexer.PeekingLexer) error {
	open := lex.Peek()
	if open.Value != "(" {
		// A `(` is mandatory here (right after `do` / `body :=`), so a missing
		// one is a hard error rather than a "try the next alternative" signal.
		return fmt.Errorf("expected '(' to open an AQL block, got %q", open.Value)
	}
	b.Pos = open.Pos
	lex.Next() // consume '('

	depth := 1
	var toks []lexer.Token
	for {
		t := lex.Next()
		if t.EOF() {
			return fmt.Errorf("unterminated AQL block (missing ')')")
		}
		switch t.Value {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				b.Raw = reconstructTokens(toks)
				if strings.TrimSpace(b.Raw) == "" {
					return fmt.Errorf("empty AQL block")
				}
				return nil
			}
		}
		toks = append(toks, *t)
	}
}

// reconstructTokens rebuilds source text from a token slice using offset
// adjacency: two tokens are concatenated directly when the second begins exactly
// where the first ends, otherwise a single space is inserted. This collapses
// runs of whitespace (and elided comments) but preserves token adjacency, so
// `$p`, floats like `1.5`, etc. reassemble into text the AQL lexer re-reads
// identically.
func reconstructTokens(toks []lexer.Token) string {
	var sb strings.Builder
	for i, t := range toks {
		if i > 0 {
			prev := toks[i-1]
			if t.Pos.Offset != prev.Pos.Offset+len(prev.Value) {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(t.Value)
	}
	return sb.String()
}
