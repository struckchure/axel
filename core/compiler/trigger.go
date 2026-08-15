package compiler

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
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

	// Rejects a `with` block rather than silently dropping it: a trigger body is
	// spliced into plpgsql, with no host statement to carry the CTE.
	if err := c.compileWith(stmt.With); err != nil {
		return "", err
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

// snakeCase mirrors asl.toSnakeCase (both delegate to lo.SnakeCase) for the
// standalone-function field passthrough, so a field reference resolves to the same
// column the DDL layer created.
func snakeCase(s string) string {
	return lo.SnakeCase(s)
}
