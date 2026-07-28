package asl

import (
	"fmt"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// Parse implements participle.Parseable for a leading `@name value?` function
// directive. It is invoked (via a `@@*` reference) at each position where a
// directive might start.
//
// The value is optional and matched positionally: after `@name`, the next token
// is taken as the value unless it opens the next directive (`@`) or the function
// declaration (`function`). This lets `@immutable` (valueless, followed by
// `function`) and `@language plpgsql` (valued) share one shape without a
// valueless flag swallowing the `function` keyword.
func (d *FuncDirective) Parse(lex *lexer.PeekingLexer) error {
	at := lex.Peek()
	if at.Value != "@" {
		// Not a directive — stop the `@@*` repetition without consuming.
		return participle.NextMatch
	}
	d.Pos = at.Pos
	lex.Next() // consume '@'

	name := lex.Next()
	if name.EOF() || name.Value == "" {
		return fmt.Errorf("expected directive name after '@'")
	}
	d.Name = name.Value

	if next := lex.Peek(); !next.EOF() && next.Value != "@" && next.Value != "function" {
		val := lex.Next().Value
		d.Value = &val
	}
	return nil
}
