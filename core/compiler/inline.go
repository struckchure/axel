package compiler

import (
	"fmt"
	"strings"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

// CompileInline compiles an inline AQL query — the interior of an aql`…`
// literal in a `.asl` function body — into a single-line Postgres string literal,
// quotes included, ready to drop into the surrounding SQL expression.
//
// An inline query is embedded as a literal rather than executed against a
// connection, so it must stand alone: query parameters have nothing to bind to
// and are rejected here instead of emitting SQL with dangling `$1` placeholders.
func CompileInline(src string, schema *asl.SchemaIR) (string, error) {
	stmt, err := aql.ParseString(src)
	if err != nil {
		return "", err
	}
	res, err := Compile(stmt, schema)
	if err != nil {
		return "", err
	}
	if len(res.Params) > 0 {
		return "", fmt.Errorf("query parameters ($%s) are not allowed — an inline query is embedded as a literal, with nothing to bind to", res.Params[0].Name)
	}
	// The literal lands inside a `$$ … $$` function body on one line: collapse the
	// compiler's line breaks, and double the single quotes so the literal closes
	// where it should.
	sql := strings.Join(strings.Fields(res.SQL), " ")
	if strings.Contains(sql, "$$") {
		return "", fmt.Errorf("compiled SQL contains $$, which would terminate the function body")
	}
	return "'" + strings.ReplaceAll(sql, "'", "''") + "'", nil
}
