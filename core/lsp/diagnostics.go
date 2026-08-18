package lsp

import (
	"errors"
	"sort"
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
	return append(diags, inlineAQLDiagnostics(text, ir)...)
}

// SchemaFile is one file of a schema split across several .asl files. Path is
// the filesystem path (resolver messages are attributed by it); URI is the
// document URI a go-to-definition result points at.
type SchemaFile struct {
	Path string
	URI  string
	Text string
}

// SchemaDiagnosticsIn reports problems for one document of a schema that is
// split across several files. text is the document being edited (at path), and
// others carries the current source of the remaining files. They are merged
// before resolving, so a type declared in a sibling file does not read as
// unknown here. Only problems attributable to this document are returned — the
// rest are reported against the file that owns them.
func SchemaDiagnosticsIn(path, text string, others []SchemaFile) []Diagnostic {
	if len(others) == 0 {
		return SchemaDiagnostics(text)
	}

	sf, err := asl.ParseNamed(path, []byte(text))
	if err != nil {
		return []Diagnostic{parseErrDiag(text, err)}
	}
	parsed := []*asl.SourceFile{sf}
	for _, o := range others {
		// A sibling that does not parse is reported in its own editor buffer;
		// here it just contributes nothing.
		if osf, err := asl.ParseNamed(o.Path, []byte(o.Text)); err == nil {
			parsed = append(parsed, osf)
		}
	}

	ir, err := (&asl.Resolver{}).Resolve(asl.Merge(parsed...))
	if err != nil {
		if d, ok := localDiag(path, text, err.Error()); ok {
			return []Diagnostic{d}
		}
		return nil
	}

	var diags []Diagnostic
	for _, e := range asl.Validate(ir) {
		if d, ok := localDiag(path, text, e.Error()); ok {
			diags = append(diags, d)
		}
	}
	return append(diags, localInlineAQLDiagnostics(text, ir)...)
}

// localDiag decides whether a resolve/validate message belongs to this document.
// A message is local when one of the names it quotes can be found in the text,
// or when it names this file outright (resolver messages about a redeclaration
// carry file:line:col for both sites). Otherwise it belongs to a sibling file.
func localDiag(path, text, msg string) (Diagnostic, bool) {
	rng := errorRange(text, msg)
	if rng == (Range{}) && !strings.Contains(msg, path) {
		return Diagnostic{}, false
	}
	return Diagnostic{Range: rng, Severity: SeverityError, Message: msg}, true
}

// localInlineAQLDiagnostics reports inline-AQL problems only for functions
// declared in this document, so a query broken in a sibling file is not
// reported here as well.
func localInlineAQLDiagnostics(text string, ir *asl.SchemaIR) []Diagnostic {
	return inlineAQLDiagnosticsFor(text, ir, func(name string) bool {
		return indexWord(text, name, 0) >= 0
	})
}

// inlineAQLDiagnostics compiles every aql`…` literal embedded in a function
// body, so a bad inline query is reported in the editor rather than at migration
// time. Functions are visited in name order for stable output.
func inlineAQLDiagnostics(text string, ir *asl.SchemaIR) []Diagnostic {
	return inlineAQLDiagnosticsFor(text, ir, nil)
}

// inlineAQLDiagnosticsFor is inlineAQLDiagnostics restricted to the functions
// accepted by include (nil accepts all). Compilation always sees the whole IR —
// only which functions are reported changes.
func inlineAQLDiagnosticsFor(text string, ir *asl.SchemaIR, include func(name string) bool) []Diagnostic {
	names := make([]string, 0, len(ir.Functions))
	for name, fn := range ir.Functions {
		if len(fn.InlineAQL) == 0 {
			continue
		}
		if include != nil && !include(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var diags []Diagnostic
	for _, name := range names {
		for _, src := range ir.Functions[name].InlineAQL {
			if _, err := compiler.CompileInline(src, ir); err != nil {
				msg := "inline aql: " + err.Error()
				diags = append(diags, Diagnostic{
					Range:    inlineAQLRange(text, src, msg),
					Severity: SeverityError,
					Message:  msg,
				})
			}
		}
	}
	return diags
}

// inlineAQLRange ranges the failing query in the source. The stored source has
// its whitespace collapsed by the ASL token reconstruction, so an exact match is
// only likely for single-spaced queries — fall back to the offending name.
func inlineAQLRange(text, src, msg string) Range {
	if idx := strings.Index(text, src); idx >= 0 {
		return Range{Start: OffsetToPosition(text, idx), End: OffsetToPosition(text, idx+len(src))}
	}
	return errorRange(text, msg)
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
