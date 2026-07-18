package compiler

import (
	"fmt"
	"strings"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

// CompileTriggerBody compiles an AQL statement used as a trigger or function
// body. The magic identifiers __new__ / __old__ / __subject__ / event are
// enabled; field access on the magic rows is validated against enclosing (pass
// nil for a standalone function, where it passes through un-validated). paramNames
// are the declared function parameters, referenced as $name in the body.
//
// The returned SQL is the bare statement with any trailing RETURNING clause
// removed and no terminating semicolon — the caller wraps it (e.g. in a plpgsql
// BEGIN … RETURN COALESCE(NEW, OLD); END;).
func CompileTriggerBody(stmt *aql.Statement, schema *asl.SchemaIR, enclosing *asl.ResolvedType, paramNames []string) (string, error) {
	params := make(map[string]bool, len(paramNames))
	for _, p := range paramNames {
		params[p] = true
	}
	c := &compiler{
		schema: schema,
		params: newParamCollector(),
		trig:   &triggerContext{enclosing: enclosing, params: params},
	}

	var sql string
	var err error
	switch {
	case stmt.Select != nil:
		sql, err = c.compileSelect(stmt.Select)
	case stmt.Insert != nil:
		sql, err = c.compileInsert(stmt.Insert)
	case stmt.Update != nil:
		sql, err = c.compileUpdate(stmt.Update)
	case stmt.Delete != nil:
		sql, err = c.compileDelete(stmt.Delete)
	default:
		return "", fmt.Errorf("empty trigger/function body")
	}
	if err != nil {
		return "", err
	}
	return stripReturning(sql), nil
}

// stripReturning removes a trailing "RETURNING …" clause (and terminating ;)
// from a compiled mutation. RETURNING is always the final clause, emitted on its
// own line, so matching "\nRETURNING" is unambiguous.
func stripReturning(sql string) string {
	if i := strings.LastIndex(sql, "\nRETURNING"); i >= 0 {
		return strings.TrimRight(sql[:i], " \n")
	}
	return strings.TrimRight(sql, "; \n")
}

// snakeCase mirrors asl.toSnakeCase for the standalone-function field passthrough.
func snakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + 32)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
