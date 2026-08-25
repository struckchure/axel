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
//
// An expression may embed inline AQL as an aql`…` literal, e.g.
//
//	return cron.schedule('kv-gc', '0 * * * *', aql`delete KV filter .expires_at < now()`);
//
// Those are stripped out here: Raw keeps the verbatim source (the formatter
// round-trips it), while Lowered holds the same text with each literal replaced
// by an inlineAQLMarker and AQL holds the raw query sources in order. The bridge
// in package axel compiles them to SQL string literals — this package cannot
// reach the AQL compiler without an import cycle.
type ReturnExpr struct {
	Pos     lexer.Position
	Raw     string   // reconstructed source of the expression (terminating ';' excluded)
	Lowered string   // Raw with each inline AQL literal replaced by inlineAQLMarker
	AQL     []string // raw AQL source of each inline literal, in order
}

// inlineAQLMarker stands in for one aql`…` literal inside ReturnExpr.Lowered.
// NUL can't appear in an .asl source file, so it can never collide with user text.
const inlineAQLMarker = "\x00"

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
		case "(", "{", "[":
			depth++
		case ")", "}", "]":
			depth--
		case ";":
			if depth <= 0 {
				lex.Next() // consume the terminating ';'
				r.Raw = reconstructTokens(toks)
				if strings.TrimSpace(r.Raw) == "" {
					return fmt.Errorf("empty return expression")
				}
				return r.extractInlineAQL(toks)
			}
		}
		toks = append(toks, *lex.Next())
	}
}

// extractInlineAQL fills Lowered and AQL from the captured tokens, pulling out
// every aql`…` pair. It mirrors reconstructTokens' offset-adjacency spacing so
// the surrounding SQL text is reproduced exactly as in Raw.
func (r *ReturnExpr) extractInlineAQL(toks []lexer.Token) error {
	var sb strings.Builder
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if i > 0 {
			prev := toks[i-1]
			if t.Pos.Offset != prev.Pos.Offset+len(prev.Value) {
				sb.WriteByte(' ')
			}
		}
		switch {
		case t.Value == "aql" && i+1 < len(toks) && isAQLString(toks[i+1]):
			src := strings.TrimSpace(strings.Trim(toks[i+1].Value, "`"))
			if src == "" {
				return fmt.Errorf("empty inline aql literal at %s", t.Pos)
			}
			r.AQL = append(r.AQL, src)
			sb.WriteString(inlineAQLMarker)
			i++ // consume the backtick literal too
		case isAQLString(t):
			return fmt.Errorf("backtick literal at %s must be prefixed with `aql`", t.Pos)
		default:
			sb.WriteString(t.Value)
		}
	}
	r.Lowered = sb.String()
	return nil
}

// isAQLString reports whether tok is a backtick-delimited AQL literal. Backtick
// is not part of any other ASL token, so the delimiter alone identifies it.
func isAQLString(tok lexer.Token) bool {
	return strings.HasPrefix(tok.Value, "`")
}
