package lsp

import (
	"errors"
	"strings"

	"github.com/alecthomas/participle/v2"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

// SchemaDiagnostics parses, resolves, and validates an ASL document, returning
// all problems found. Parse errors carry a precise position; resolve/validate
// errors are attached to the named declaration when it can be located, else to
// the start of the file.
func SchemaDiagnostics(text string) []Diagnostic {
	sf, err := asl.Parse([]byte(text))
	if err != nil {
		return []Diagnostic{parseErrDiag(text, err)}
	}
	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		return []Diagnostic{{
			Range:    errorRange(text, err.Error()),
			Severity: SeverityError,
			Message:  err.Error(),
		}}
	}
	var diags []Diagnostic
	for _, e := range asl.Validate(ir) {
		diags = append(diags, Diagnostic{
			Range:    errorRange(text, e.Error()),
			Severity: SeverityError,
			Message:  e.Error(),
		})
	}
	return diags
}

// QueryDiagnostics parses an AQL document and, when a resolved schema is
// available, compiles it against that schema to surface unknown-type / unknown-
// field errors. With a nil schema it reports parse errors only.
func QueryDiagnostics(text string, schema *asl.SchemaIR) []Diagnostic {
	stmt, err := aql.ParseString(text)
	if err != nil {
		return []Diagnostic{parseErrDiag(text, err)}
	}
	if schema == nil {
		return nil
	}
	if _, err := compiler.Compile(stmt, schema); err != nil {
		msg := err.Error()
		rng := statementRange(text, stmt)
		// Prefer a range around the most specific (innermost) name in the message.
		names := quotedNames(msg)
		for i := len(names) - 1; i >= 0; i-- {
			if idx := indexWord(text, names[i], 0); idx >= 0 {
				rng = Range{Start: OffsetToPosition(text, idx), End: OffsetToPosition(text, idx+len(names[i]))}
				break
			}
		}
		return []Diagnostic{{Range: rng, Severity: SeverityError, Message: msg}}
	}
	return nil
}

// parseErrDiag turns a participle parse error into a positioned diagnostic.
func parseErrDiag(text string, err error) Diagnostic {
	var pErr participle.Error
	if errors.As(err, &pErr) {
		return Diagnostic{
			Range:    wordRange(text, pErr.Position().Offset),
			Severity: SeverityError,
			Message:  pErr.Message(),
		}
	}
	return Diagnostic{Severity: SeverityError, Message: err.Error()}
}

// errorRange best-effort ranges the offending symbol named in a resolve/validate
// message. Messages nest outer→inner (e.g. type "User": property "role": unknown
// type "Nope"), so the most specific name is last — try names in reverse and
// range the first one found in the source; fall back to the file start.
func errorRange(text, msg string) Range {
	names := quotedNames(msg)
	for i := len(names) - 1; i >= 0; i-- {
		if idx := indexWord(text, names[i], 0); idx >= 0 {
			return Range{Start: OffsetToPosition(text, idx), End: OffsetToPosition(text, idx+len(names[i]))}
		}
	}
	return Range{}
}

func statementRange(text string, stmt *aql.Statement) Range {
	start, end := stmt.Pos.Offset, stmt.EndPos.Offset
	if end <= start {
		end = start + 1
	}
	return Range{Start: OffsetToPosition(text, start), End: OffsetToPosition(text, end)}
}

// indexWord returns the byte index of name occurring as a whole word at or after
// `from`, or -1.
func indexWord(text, name string, from int) int {
	if name == "" || from < 0 {
		return -1
	}
	for i := from; i+len(name) <= len(text); i++ {
		if text[i:i+len(name)] != name {
			continue
		}
		if i > 0 && isWordByte(text[i-1]) {
			continue
		}
		if i+len(name) < len(text) && isWordByte(text[i+len(name)]) {
			continue
		}
		return i
	}
	return -1
}

// quotedNames returns the substrings inside double quotes in msg.
func quotedNames(msg string) []string {
	var out []string
	for {
		i := strings.IndexByte(msg, '"')
		if i < 0 {
			break
		}
		rest := msg[i+1:]
		j := strings.IndexByte(rest, '"')
		if j < 0 {
			break
		}
		out = append(out, rest[:j])
		msg = rest[j+1:]
	}
	return out
}
