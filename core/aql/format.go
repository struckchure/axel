package aql

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// Format parses an AQL statement and returns it re-printed in canonical form,
// preserving `#` comments. It is safe: if the reformatted text fails to re-parse
// or does not render back to the same structure as the input, the original
// source is returned unchanged.
func Format(src []byte) (string, error) {
	stmt, err := Parse(src)
	if err != nil {
		return "", err
	}
	out := formatStmt(string(src), stmt)

	stmt2, err := Parse([]byte(out))
	if err != nil || Print(stmt) != Print(stmt2) {
		return string(src), nil
	}
	return out, nil
}

// formatStmt renders a statement with comments from origSrc reattached.
func formatStmt(origSrc string, stmt *Statement) string {
	f := &aqlFmt{cmts: collectComments(origSrc)}
	f.statement(stmt)
	return f.done()
}

// aqlFmt accumulates formatted output line by line, mirroring the ASL formatter,
// so trailing comments can be appended to the line they belong to.
type aqlFmt struct {
	out    strings.Builder
	cur    strings.Builder
	indent int
	// hang is an extra indent level applied to committed lines. A boolean chain
	// broken without wrapping parens uses it so its continuations read as
	// subordinate to `filter` rather than as new clauses:
	//
	//	filter .sender_id = $a
	//	  and .reciever_id = $b
	//
	// It is set by expr/andExpr at their first break and cleared by clauseBreak,
	// which every clause and statement boundary goes through.
	hang int
	cmts []comment
	ci   int
}

type comment struct {
	Text   string
	Offset int
	Own    bool
}

// maxWidth is the column budget the formatter keeps lines within. It governs
// only boolean chains: one that fits stays on a single line, a longer one is
// broken at its outermost connective (see expr).
const maxWidth = 80

func (f *aqlFmt) w(s string)                 { f.cur.WriteString(s) }
func (f *aqlFmt) wf(format string, a ...any) { fmt.Fprintf(&f.cur, format, a...) }

// fits reports whether appending s to the line under construction keeps it
// within maxWidth. f.cur holds the line without its indent prefix — commit
// writes the indent ahead of it — so the prefix is counted explicitly here.
func (f *aqlFmt) fits(s string) bool {
	return f.indent*2+f.cur.Len()+len(s) <= maxWidth
}

// oneLine renders a node through the canonical single-line printer so its width
// can be measured before deciding whether to break.
func oneLine(write func(*strings.Builder)) string {
	var b strings.Builder
	write(&b)
	return b.String()
}

func (f *aqlFmt) commit(trailingBefore int) {
	for f.ci < len(f.cmts) && !f.cmts[f.ci].Own && f.cmts[f.ci].Offset < trailingBefore {
		f.cur.WriteString("  " + f.cmts[f.ci].Text)
		f.ci++
	}
	if f.cur.Len() > 0 {
		f.out.WriteString(strings.Repeat("  ", f.indent+f.hang))
		f.out.WriteString(f.cur.String())
	}
	f.out.WriteByte('\n')
	f.cur.Reset()
}

// clauseBreak commits the current line and drops any hanging indent, so the next
// clause starts back at the statement's own level.
func (f *aqlFmt) clauseBreak(trailingBefore int) {
	f.commit(trailingBefore)
	f.hang = 0
}

func (f *aqlFmt) leading(offset int) {
	for f.ci < len(f.cmts) && f.cmts[f.ci].Offset < offset {
		f.out.WriteString(strings.Repeat("  ", f.indent+f.hang))
		f.out.WriteString(f.cmts[f.ci].Text)
		f.out.WriteByte('\n')
		f.ci++
	}
}

func (f *aqlFmt) done() string {
	f.leading(1 << 30)
	s := strings.TrimLeft(f.out.String(), "\n")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimRight(s, "\n") + "\n"
}

func (f *aqlFmt) statement(stmt *Statement) {
	f.leading(stmt.Pos.Offset)
	for _, d := range stmt.Directives {
		f.wf("@%s %s", d.Name, d.Value)
		f.commit(d.EndPos.Offset)
	}
	if len(stmt.Directives) > 0 {
		f.out.WriteByte('\n')
	}
	for _, v := range stmt.Vars {
		f.varBlock(v)
	}
	if len(stmt.Vars) > 0 {
		f.out.WriteByte('\n')
	}
	f.withBlock(stmt.With)
	switch {
	case stmt.Select != nil:
		f.selectStmt(stmt.Select)
	case stmt.Insert != nil:
		f.insertStmt(stmt.Insert)
	case stmt.Update != nil:
		f.updateStmt(stmt.Update)
	case stmt.Delete != nil:
		f.deleteStmt(stmt.Delete)
	}
}

// varBlock renders `var ( ... )` or `var $param...;`.
func (f *aqlFmt) varBlock(v *VarBlock) {
	if v == nil || len(v.Params) == 0 {
		return
	}
	if len(v.Params) == 1 && v.Pos.Line == v.EndPos.Line {
		f.leading(v.Pos.Offset)
		f.w("var ")
		printParam(&f.cur, v.Params[0])
		f.w(";")
		f.clauseBreak(v.EndPos.Offset)
		return
	}
	f.leading(v.Pos.Offset)
	f.w("var (")
	first := 1 << 30
	if len(v.Params) > 0 {
		first = v.Params[0].Pos.Offset
	}
	f.clauseBreak(first)
	f.indent++
	for i, p := range v.Params {
		f.leading(p.Pos.Offset)
		printParam(&f.cur, p)
		f.w(";")
		next := 1 << 30
		if i+1 < len(v.Params) {
			next = v.Params[i+1].Pos.Offset
		}
		f.clauseBreak(next)
	}
	f.indent--
	f.w(")")
	f.clauseBreak(v.EndPos.Offset)
}

// withBlock renders `with ( name := expr; … )` with one binding per indented
// line. When a binding's value is a solo subquery (the common case), it is
// rendered with the same clause-per-line treatment as a top-level select,
// wrapped in indented parens:
//
//	with (
//	  api_key := (
//	    multi select ApiKey { id }
//	    filter .id = $id<uuid>?
//	  );
//	)
//
// Non-subquery bindings (rare) are rendered inline on the same line as `:=`.
func (f *aqlFmt) withBlock(w *WithBlock) {
	if w == nil {
		return
	}
	f.w("with (")
	first := 1 << 30
	if len(w.Bindings) > 0 {
		first = w.Bindings[0].Pos.Offset
	}
	f.clauseBreak(first)
	f.indent++
	for i, b := range w.Bindings {
		f.leading(b.Pos.Offset)
		f.wf("%s := ", b.Name)
		f.withBindingValue(b.Value)
		f.w(";")
		next := 1 << 30
		if i+1 < len(w.Bindings) {
			next = w.Bindings[i+1].Pos.Offset
		}
		f.clauseBreak(next)
	}
	f.indent--
	f.w(")")
	f.clauseBreak(w.EndPos.Offset)
}

// withBindingValue renders the right-hand side of a with-binding. When the
// value is a solo subquery (the overwhelmingly common case) it is formatted
// like a top-level select: opening paren on the current line, each clause on
// its own indented line, closing paren de-indented. Everything else is inlined
// via the single-line printer.
func (f *aqlFmt) withBindingValue(val *Expr) {
	p := val.SoloPrimary()
	if p == nil || p.SubQuery == nil {
		// Not a solo subquery — render inline.
		printExpr(&f.cur, val)
		return
	}
	body := p.SubQuery

	// Try to fit the whole thing on the current line first.
	inline := oneLine(func(b *strings.Builder) {
		b.WriteString("(")
		if p.SubQueryMulti {
			b.WriteString("multi ")
		}
		b.WriteString("select ")
		printSelectBodyInline(b, body)
		b.WriteString(")")
	})
	if f.fits(inline) {
		f.w(inline)
		return
	}

	// Doesn't fit: open paren, then render each clause on its own indented line.
	f.w("(")
	f.clauseBreak(1 << 30)
	f.indent++

	// Header: [multi] select TypeName [{ shape }]
	if p.SubQueryMulti {
		f.w("multi ")
	}
	f.w("select ")
	f.w(body.TypeName)
	if body.Shape != nil {
		f.w(" ")
		f.shape(body.Shape)
	}

	// filter / order by / limit / offset — each on its own line.
	if body.Filter != nil {
		f.clauseBreak(1 << 30)
		f.w("filter ")
		f.expr(body.Filter.Expr, true)
	}
	for i, o := range body.OrderBy {
		if i == 0 {
			f.clauseBreak(1 << 30)
			f.w("order by ")
		} else {
			f.w(", ")
		}
		printExpr(&f.cur, o.Expr)
		if o.Dir != "" {
			f.wf(" %s", o.Dir)
		}
	}
	if body.Limit != nil {
		f.clauseBreak(1 << 30)
		f.w("limit ")
		printExpr(&f.cur, body.Limit)
	}
	if body.Offset != nil {
		f.clauseBreak(1 << 30)
		f.w("offset ")
		printExpr(&f.cur, body.Offset)
	}

	f.clauseBreak(1 << 30)
	f.indent--
	f.w(")")
}


func (f *aqlFmt) selectStmt(s *SelectStmt) {
	if s.Multi {
		f.w("multi ")
	}
	f.w("select ")
	f.selectBody(s.Body)
	f.w(";")
	f.clauseBreak(1 << 30)
}

// selectBody renders an aggregate or object select. The object form may open a
// multi-line shape; clauses (filter/order/limit/offset) follow on their own
// lines. The caller appends the terminating ';' to the final line.
func (f *aqlFmt) selectBody(body *SelectBody) {
	if body.AggFunc != nil {
		f.wf("%s(%s", body.AggFunc.Func, body.AggFunc.TypeName)
		if body.AggFunc.Filter != nil {
			f.w(" ")
			printFilter(&f.cur, body.AggFunc.Filter)
		}
		f.w(")")
		return
	}
	f.w(body.TypeName)
	if body.Shape != nil {
		f.w(" ")
		f.shape(body.Shape)
	}
	if body.Filter != nil {
		f.newlineClause()
		f.w("filter ")
		f.expr(body.Filter.Expr, true)
	}
	for i, g := range body.GroupBy {
		if i == 0 {
			f.newlineClause()
			f.w("group by ")
		} else {
			f.w(", ")
		}
		printExpr(&f.cur, g.Expr)
	}
	if body.Having != nil {
		f.newlineClause()
		f.w("having ")
		f.expr(body.Having.Expr, true)
	}
	for i, o := range body.OrderBy {
		if i == 0 {
			f.newlineClause()
			f.w("order by ")
		} else {
			f.w(", ")
		}
		printExpr(&f.cur, o.Expr)
		if o.Dir != "" {
			f.wf(" %s", o.Dir)
		}
	}
	if body.Limit != nil {
		f.newlineClause()
		f.w("limit ")
		printExpr(&f.cur, body.Limit)
	}
	if body.Offset != nil {
		f.newlineClause()
		f.w("offset ")
		printExpr(&f.cur, body.Offset)
	}
}

// newlineClause commits the current line and starts the next clause at indent 0.
func (f *aqlFmt) newlineClause() { f.clauseBreak(1 << 30) }

// inlineFilter renders the `filter <expr>` tail of an update/delete, which
// shares a line with the statement head. When the whole line would overflow the
// filter moves to its own line, giving those statements the same shape select
// already has.
func (f *aqlFmt) inlineFilter(flt *Filter) {
	if flt == nil {
		return
	}
	if s := oneLine(func(b *strings.Builder) { printFilter(b, flt) }); f.fits(" " + s) {
		f.w(" " + s)
		return
	}
	f.commit(flt.Expr.Pos.Offset)
	f.w("filter ")
	f.expr(flt.Expr, true)
}

// expr renders a boolean expression, breaking it across lines only when the
// single-line form would overflow maxWidth. Arms after the first lead with their
// connective on a fresh line at the current indent:
//
//	filter (
//	  business is not null
//	  and (
//	    .sender_id = business.id
//	    or .sender_id in api_keys.id
//	  )
//	)
//
// Breaking never changes the AST — no parens are added or removed — so Format's
// re-parse check still proves the reformat was lossless.
//
// hang requests the subordinate indent described on aqlFmt.hang. It is set by
// the filter call sites and cleared inside a parenthesized group, which supplies
// its own indentation.
func (f *aqlFmt) expr(e *Expr, hang bool) {
	if e == nil {
		return
	}
	if s := oneLine(func(b *strings.Builder) { printExpr(b, e) }); f.fits(s) {
		f.w(s)
		return
	}
	f.andExpr(e.Left, hang)
	for _, a := range e.Rest {
		// Commit the preceding line first, then hang: the first line of a chain
		// sits at the clause's own level, only its continuations are indented.
		f.commit(a.Pos.Offset)
		if hang {
			f.hang = 1
		}
		f.leading(a.Pos.Offset)
		f.w("or ")
		f.andExpr(a, hang)
	}
}

// andExpr is expr's `and` level: same try-inline-then-break shape, one
// precedence step down.
func (f *aqlFmt) andExpr(a *AndExpr, hang bool) {
	if a == nil {
		return
	}
	if s := oneLine(func(b *strings.Builder) { printAndExpr(b, a) }); f.fits(s) {
		f.w(s)
		return
	}
	f.cmp(a.Left)
	for _, c := range a.Rest {
		f.commit(c.Pos.Offset)
		if hang {
			f.hang = 1
		}
		f.leading(c.Pos.Offset)
		f.w("and ")
		f.cmp(c)
	}
}

// cmp renders one comparison. The only comparison the formatter may open across
// lines is a bare parenthesized group, whose interior is re-entered at one more
// indent; everything else — a comparison, a null test, a subquery operand — is
// atomic and stays on its line even when it overflows.
func (f *aqlFmt) cmp(c *Cmp) {
	if c == nil {
		return
	}
	s := oneLine(func(b *strings.Builder) { printCmp(b, c) })
	if f.fits(s) {
		f.w(s)
		return
	}
	g := groupOperand(c)
	if g == nil {
		f.w(s)
		return
	}
	f.hang = 0
	f.w("(")
	f.clauseBreak(g.Pos.Offset)
	f.indent++
	f.leading(g.Pos.Offset)
	f.expr(g, false)
	// Commit the group's last line while still at the inner indent, so the
	// closing paren lands back at the parent's.
	f.clauseBreak(g.EndPos.Offset)
	f.indent--
	f.w(")")
}

// groupOperand returns the interior of a comparison that is nothing but a
// parenthesized expression — the one shape that can be opened across lines. A
// cast or an operator makes the parens part of a larger operand, so those stay
// inline.
func groupOperand(c *Cmp) *Expr {
	if c.Is || c.Op != "" || c.Left == nil {
		return nil
	}
	if c.Left.SubExpr == nil || c.Left.Cast != "" {
		return nil
	}
	return c.Left.SubExpr
}

func (f *aqlFmt) shape(s *Shape) {
	// Try to keep the shape on a single line: `{ id, name }`.
	if inline := oneLine(func(b *strings.Builder) { printShapeInline(b, s) }); f.fits(inline) {
		f.w(inline)
		return
	}
	// Doesn't fit: multi-line with one field per indented line.
	f.w("{")
	first := 1 << 30
	if len(s.Fields) > 0 {
		first = s.Fields[0].Pos.Offset
	}
	f.clauseBreak(first)
	f.indent++
	for i, fld := range s.Fields {
		f.leading(fld.Pos.Offset)
		f.shapeField(fld)
		if i < len(s.Fields)-1 {
			f.w(",")
		}
		next := 1 << 30
		if i+1 < len(s.Fields) {
			next = s.Fields[i+1].Pos.Offset
		}
		f.clauseBreak(next)
	}
	f.indent--
	f.w("}")
}


func (f *aqlFmt) shapeField(fld *ShapeField) {
	if fld.Star {
		f.w("*")
	} else {
		f.w(fld.Name)
	}
	if fld.SubShape != nil {
		f.w(": ")
		f.shape(fld.SubShape)
	}
	if fld.Computed != nil {
		f.w(" := ")
		printExpr(&f.cur, fld.Computed)
	}
	if fld.AggFilter != nil {
		f.w(" ")
		printFilter(&f.cur, fld.AggFilter)
	}
}

func (f *aqlFmt) insertStmt(s *InsertStmt) {
	f.wf("insert %s {", s.TypeName)
	f.assignmentBlock(s.Assignments)
	f.w("}")
	printConflict(&f.cur, s.Conflict)
	f.w(";")
	f.clauseBreak(1 << 30)
}

func (f *aqlFmt) updateStmt(s *UpdateStmt) {
	f.wf("update %s", s.TypeName)
	f.inlineFilter(s.Filter)
	f.clauseBreak(1 << 30)
	f.w("set {")
	f.assignmentBlock(s.Assignments)
	f.w("};")
	f.clauseBreak(1 << 30)
}

func (f *aqlFmt) deleteStmt(s *DeleteStmt) {
	f.wf("delete %s", s.TypeName)
	f.inlineFilter(s.Filter)
	f.w(";")
	f.clauseBreak(1 << 30)
}

// assignmentBlock renders `field := expr` entries, one per indented line, between
// an already-open `{` (on the current line) and a `}` the caller writes next.
func (f *aqlFmt) assignmentBlock(as []*Assignment) {
	first := 1 << 30
	if len(as) > 0 {
		first = as[0].Pos.Offset
	}
	f.clauseBreak(first)
	f.indent++
	for i, a := range as {
		f.leading(a.Pos.Offset)
		f.wf("%s := ", a.Field)
		if a.LinkDelta != nil {
			f.w("{\n")
			f.indent++
			for j, item := range a.LinkDelta.Items {
				f.leading(item.Pos.Offset)
				op := item.NormalizedOp()
				f.wf("%q: ", op)
				printExpr(&f.cur, item.Value)
				if j < len(a.LinkDelta.Items)-1 {
					f.w(",")
				}
				itemNext := 1 << 30
				if j+1 < len(a.LinkDelta.Items) {
					itemNext = a.LinkDelta.Items[j+1].Pos.Offset
				}
				f.clauseBreak(itemNext)
			}
			f.indent--
			f.w("}")
		} else {
			printExpr(&f.cur, a.Value)
		}
		if i < len(as)-1 {
			f.w(",")
		}
		next := 1 << 30
		if i+1 < len(as) {
			next = as[i+1].Pos.Offset
		}
		f.clauseBreak(next)
	}
	f.indent--
}

// collectComments lexes src and returns its `#` comments in source order.
func collectComments(src string) []comment {
	if src == "" {
		return nil
	}
	lex, err := aqlLexer.Lex("", strings.NewReader(src))
	if err != nil {
		return nil
	}
	toks, err := lexer.ConsumeAll(lex)
	if err != nil {
		return nil
	}
	ct, ok := aqlLexer.Symbols()["Comment"]
	if !ok {
		return nil
	}
	var out []comment
	for _, t := range toks {
		if t.Type != ct {
			continue
		}
		out = append(out, comment{
			Text:   strings.TrimRight(t.Value, " \t\r"),
			Offset: t.Pos.Offset,
			Own:    ownLine(src, t.Pos.Offset),
		})
	}
	return out
}

func ownLine(src string, offset int) bool {
	for i := offset - 1; i >= 0 && src[i] != '\n'; i-- {
		if src[i] != ' ' && src[i] != '\t' && src[i] != '\r' {
			return false
		}
	}
	return true
}
