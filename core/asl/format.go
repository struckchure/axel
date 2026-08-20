package asl

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// Format parses an .asl schema and returns it re-printed in canonical form,
// preserving `#` comments. It is safe: if the reformatted text fails to re-parse
// or does not render back to the same structure as the input, the original
// source is returned unchanged rather than risk corrupting the file.
func Format(src []byte) (string, error) {
	sf, err := Parse(src)
	if err != nil {
		return "", err
	}
	out := render(string(src), sf)

	// Safety net: the formatted output must re-parse to the same structure
	// (comments aside). If it doesn't, leave the file untouched.
	sf2, err := Parse([]byte(out))
	if err != nil || render("", sf) != render("", sf2) {
		return string(src), nil
	}
	return out, nil
}

// render pretty-prints a SourceFile. When origSrc is non-empty, `#` comments from
// it are collected and re-attached; passing "" renders without comments (used for
// the structural-equivalence check in Format).
func render(origSrc string, sf *SourceFile) string {
	f := &aslFmt{}
	if origSrc != "" {
		f.cmts = collectComments(origSrc)
	}
	f.file(sf)
	return f.done()
}

// aslFmt accumulates formatted output line by line so a trailing comment can be
// appended to the line it belongs to before the newline is written.
type aslFmt struct {
	out    strings.Builder
	cur    strings.Builder // content of the line being built (no indent, no newline)
	indent int
	cmts   []comment
	ci     int // cursor into cmts
}

// comment is a `#` comment captured from source. Own is true when only
// whitespace precedes it on its line (a leading/own-line comment); false marks a
// trailing comment sharing a line with code.
type comment struct {
	Text   string
	Offset int
	Line   int
	Own    bool
}

func (f *aslFmt) w(s string)                 { f.cur.WriteString(s) }
func (f *aslFmt) wf(format string, a ...any) { fmt.Fprintf(&f.cur, format, a...) }

// commit ends the current logical line. trailingBefore is the source offset just
// past this line's content: any trailing (non-own-line) comment before it is
// appended inline.
func (f *aslFmt) commit(trailingBefore int) {
	for f.ci < len(f.cmts) && !f.cmts[f.ci].Own && f.cmts[f.ci].Offset < trailingBefore {
		f.cur.WriteString("  " + f.cmts[f.ci].Text)
		f.ci++
	}
	if f.cur.Len() > 0 {
		f.out.WriteString(strings.Repeat("  ", f.indent))
		f.out.WriteString(f.cur.String())
	}
	f.out.WriteByte('\n')
	f.cur.Reset()
}

// blank writes a single empty line.
func (f *aslFmt) blank() { f.out.WriteByte('\n') }

// leading emits own-line comments occurring before offset, each on its own line
// at the current indent.
func (f *aslFmt) leading(offset int) {
	for f.ci < len(f.cmts) && f.cmts[f.ci].Offset < offset {
		if !f.cmts[f.ci].Own {
			// A stray trailing comment with no line to attach to: emit standalone.
			f.out.WriteString(strings.Repeat("  ", f.indent))
			f.out.WriteString(f.cmts[f.ci].Text)
			f.out.WriteByte('\n')
			f.ci++
			continue
		}
		f.out.WriteString(strings.Repeat("  ", f.indent))
		f.out.WriteString(f.cmts[f.ci].Text)
		f.out.WriteByte('\n')
		f.ci++
	}
}

func (f *aslFmt) done() string {
	// Flush any comments trailing the final declaration.
	f.leading(1 << 30)
	s := f.out.String()
	// Collapse 3+ newlines to a single blank line and trim leading/trailing space.
	s = strings.TrimLeft(s, "\n")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	s = strings.TrimRight(s, "\n") + "\n"
	return s
}

func (f *aslFmt) file(sf *SourceFile) {
	for i, d := range sf.Definitions {
		if i > 0 {
			f.blank()
		}
		f.leading(defOffset(d))
		next := 1 << 30
		if i+1 < len(sf.Definitions) {
			next = defOffset(sf.Definitions[i+1])
		}
		f.definition(d, next)
	}
}

func defOffset(d *Definition) int {
	switch {
	case d.ScalarType != nil:
		return d.ScalarType.Pos.Offset
	case d.EnumType != nil:
		return d.EnumType.Pos.Offset
	case d.Extension != nil:
		return d.Extension.Pos.Offset
	case d.Global != nil:
		return d.Global.Pos.Offset
	case d.Function != nil:
		return d.Function.Pos.Offset
	case d.TypeDef != nil:
		return d.TypeDef.Pos.Offset
	}
	return 0
}

func (f *aslFmt) definition(d *Definition, next int) {
	switch {
	case d.Extension != nil:
		f.wf("use extension %s;", d.Extension.Name)
		f.commit(next)
	case d.Global != nil:
		g := d.Global
		f.w("global ")
		if g.Required {
			f.w("required ")
		}
		f.wf("%s: %s;", g.Name, g.Type)
		f.commit(next)
	case d.ScalarType != nil:
		f.scalarTypeDef(d.ScalarType, next)
	case d.EnumType != nil:
		e := d.EnumType
		f.wf("enum %s { %s }", e.Name, strings.Join(e.Values, ", "))
		f.commit(next)
	case d.Function != nil:
		f.function(d.Function, next)
	case d.TypeDef != nil:
		f.typeDef(d.TypeDef, next)
	}
}

func (f *aslFmt) scalarTypeDef(s *ScalarTypeDef, next int) {
	if s.Body == nil || len(s.Body.Fields) == 0 {
		f.wf("scalar type %s extends %s;", s.Name, s.Extends)
		f.commit(next)
		return
	}
	f.wf("scalar type %s extends %s {", s.Name, s.Extends)
	if len(s.Body.Fields) > 0 {
		f.commit(s.Body.Fields[0].Pos.Offset)
	} else {
		f.commit(next)
	}
	f.indent++
	for i, field := range s.Body.Fields {
		f.leading(field.Pos.Offset)
		fieldNext := next
		if i+1 < len(s.Body.Fields) {
			fieldNext = s.Body.Fields[i+1].Pos.Offset
		}
		req := ""
		if field.Required {
			req = "required "
		}
		multi := ""
		if field.Multi {
			multi = "multi "
		}
		f.wf("%s%s%s: %s;", req, multi, field.Name, field.Type)
		f.commit(fieldNext)
	}
	f.indent--
	f.leading(s.EndPos.Offset)
	f.w("}")
	f.commit(next)
}

func (f *aslFmt) function(fn *FunctionDecl, next int) {
	for _, dir := range fn.Directives {
		if dir.Value != nil {
			f.wf("@%s %s", dir.Name, *dir.Value)
		} else {
			f.wf("@%s", dir.Name)
		}
		f.commit(fn.Pos.Offset)
	}
	params := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		arr := ""
		if p.Array {
			arr = "[]"
		}
		params[i] = fmt.Sprintf("%s: %s%s", p.Name, p.Type, arr)
	}
	ret := fn.Returns
	if fn.ReturnArray {
		ret += "[]"
	}
	body := ""
	if fn.Return != nil {
		body = fn.Return.Raw
	}
	f.wf("function %s(%s) -> %s { return %s; }", fn.Name, strings.Join(params, ", "), ret, body)
	f.commit(next)
}

func (f *aslFmt) typeDef(t *TypeDef, next int) {
	if t.Abstract {
		f.w("abstract ")
	}
	f.wf("type %s", t.Name)
	if len(t.Extending) > 0 {
		f.wf(" extends %s", strings.Join(t.Extending, ", "))
	}
	if len(t.Members) == 0 {
		f.w(" {}")
		f.commit(next)
		return
	}
	f.w(" {")
	// Header line trailing comment attaches before the first member.
	f.commit(memberOffset(t.Members[0]))
	f.indent++

	// Comments are bucketed per member before anything is printed: members are
	// re-ordered below, so a single source-order cursor would hand a comment to
	// whichever member happened to be printed at that moment.
	slots, tail := f.splitBodyComments(t)

	prevBlock := -1
	for _, i := range orderedMembers(t.Members) {
		m := t.Members[i]
		if b := blockOf(m); b != prevBlock {
			if prevBlock >= 0 {
				f.blank()
			}
			prevBlock = b
		}
		end := t.EndPos.Offset
		if i+1 < len(t.Members) {
			end = memberOffset(t.Members[i+1])
		}
		f.withComments(slots[i], func() {
			f.leading(memberOffset(m))
			f.member(m, end)
		})
	}
	for _, c := range tail {
		f.out.WriteString(strings.Repeat("  ", f.indent))
		f.out.WriteString(c.Text)
		f.out.WriteByte('\n')
	}

	f.indent--
	f.w("}")
	f.commit(next)
}

// Member blocks, in printed order. Properties, links and computed fields all
// describe the row, so they share one block; each of the remaining kinds is
// printed as its own block, separated by a blank line.
const (
	blockRow        = 0 // properties and links
	blockConstraint = 1
	blockIndex      = 2
	blockPolicy     = 3
	blockTrigger    = 4
)

// blockOf returns the block a member is printed in.
func blockOf(m *Member) int {
	switch {
	case m.Constraint != nil:
		return blockConstraint
	case m.Index != nil:
		return blockIndex
	case m.Policy != nil:
		return blockPolicy
	case m.Trigger != nil:
		return blockTrigger
	default: // Field, Computed
		return blockRow
	}
}

// orderedMembers returns member indices grouped by block, and — within the row
// block — properties and links before computed fields. Order inside each group
// is the order they were written in. Field order is what decides column order,
// and every other kind is diffed by name, so this only moves lines around.
func orderedMembers(members []*Member) []int {
	order := make([]int, 0, len(members))
	rank := func(m *Member) int {
		if b := blockOf(m); b != blockRow {
			return b + 1
		}
		if m.Computed != nil {
			return 1 // computed close the row block
		}
		return 0
	}
	for want := 0; want <= blockTrigger+1; want++ {
		for i, m := range members {
			if rank(m) == want {
				order = append(order, i)
			}
		}
	}
	return order
}

// splitBodyComments consumes every comment inside the type body and buckets it
// by the member it belongs to: an own-line comment leads the member that follows
// it, a trailing comment stays with the member whose line it shares. Comments
// after the last member are returned separately, to be printed before the
// closing brace.
func (f *aslFmt) splitBodyComments(t *TypeDef) ([][]comment, []comment) {
	slots := make([][]comment, len(t.Members))
	var tail []comment
	for f.ci < len(f.cmts) && f.cmts[f.ci].Offset < t.EndPos.Offset {
		c := f.cmts[f.ci]
		f.ci++

		// A comment written inside a member (in a field body, a policy predicate)
		// belongs to that member, wherever it ends up being printed.
		if i, ok := memberContaining(t.Members, c.Offset); ok {
			slots[i] = append(slots[i], c)
			continue
		}
		next := -1
		for i, m := range t.Members {
			if memberOffset(m) > c.Offset {
				next = i
				break
			}
		}
		switch {
		case c.Own && next >= 0:
			slots[next] = append(slots[next], c)
		case c.Own:
			tail = append(tail, c)
		case next > 0:
			slots[next-1] = append(slots[next-1], c)
		case next < 0 && len(t.Members) > 0:
			slots[len(t.Members)-1] = append(slots[len(t.Members)-1], c)
		default:
			tail = append(tail, c)
		}
	}
	return slots, tail
}

// memberContaining returns the member whose source span covers offset. Only
// members that can hold a nested comment carry an end position; the single-line
// kinds (index, constraint, computed) cannot contain one.
func memberContaining(members []*Member, offset int) (int, bool) {
	for i, m := range members {
		end := mEndOffset(m)
		if end > 0 && offset > memberOffset(m) && offset < end {
			return i, true
		}
	}
	return 0, false
}

func mEndOffset(m *Member) int {
	switch {
	case m.Field != nil:
		return m.Field.EndPos.Offset
	case m.Policy != nil:
		return m.Policy.EndPos.Offset
	case m.Trigger != nil:
		return m.Trigger.EndPos.Offset
	}
	return 0
}

// withComments runs fn against a private comment cursor, so a member printed out
// of source order only ever sees its own comments. Anything fn did not place is
// flushed after it: a misplaced comment is a nuisance, a dropped one is data
// loss.
func (f *aslFmt) withComments(cs []comment, fn func()) {
	saveCmts, saveCi := f.cmts, f.ci
	f.cmts, f.ci = cs, 0
	fn()
	for ; f.ci < len(f.cmts); f.ci++ {
		f.out.WriteString(strings.Repeat("  ", f.indent))
		f.out.WriteString(f.cmts[f.ci].Text)
		f.out.WriteByte('\n')
	}
	f.cmts, f.ci = saveCmts, saveCi
}

func mPosLine(m *Member) int { return mPos(m).Line }

func mPos(m *Member) lexer.Position {
	switch {
	case m.Computed != nil:
		return m.Computed.Pos
	case m.Index != nil:
		return m.Index.Pos
	case m.Constraint != nil:
		return m.Constraint.Pos
	case m.Trigger != nil:
		return m.Trigger.Pos
	case m.Policy != nil:
		return m.Policy.Pos
	case m.Field != nil:
		return m.Field.Pos
	}
	return lexer.Position{}
}

func memberOffset(m *Member) int { return mPos(m).Offset }

func (f *aslFmt) member(m *Member, next int) {
	switch {
	case m.Field != nil:
		f.field(m.Field, next)
	case m.Computed != nil:
		c := m.Computed
		f.wf("computed %s := %s;", c.Name, joinTokens(c.Parts))
		f.commit(next)
	case m.Index != nil:
		f.wf("index on (%s);", dottedFields(m.Index.Fields))
		f.commit(next)
	case m.Constraint != nil:
		c := m.Constraint
		f.wf("constraint %s", c.Expression)
		if len(c.Args) > 0 {
			f.wf("(%s)", strings.Join(c.Args, ", "))
		}
		f.wf(" on (%s)", dottedFields(c.Fields))
		if c.Filter != nil {
			f.wf(" filter %s", c.Filter.AQL())
		}
		f.w(";")
		f.commit(next)
	case m.Trigger != nil:
		f.trigger(m.Trigger, next)
	case m.Policy != nil:
		f.policy(m.Policy, next)
	}
}

func (f *aslFmt) field(fd *FieldDecl, next int) {
	for _, kw := range []struct {
		on   bool
		word string
	}{
		{fd.Required, "required"},
		{fd.Multi, "multi"},
		{fd.Single, "single"},
		{fd.PropKeyword, "property"},
		{fd.LinkKeyword, "link"},
	} {
		if kw.on {
			f.w(kw.word + " ")
		}
	}
	f.w(fd.Name)
	if fd.TypeSpec != nil && fd.TypeSpec.PropType != nil {
		f.wf(": %s", *fd.TypeSpec.PropType)
	}
	if fd.Body == nil || len(fd.Body.Items) == 0 {
		f.w(";")
		f.commit(next)
		return
	}
	f.w(" {")
	f.commit(fieldBodyItemOffset(fd.Body.Items[0], next))
	f.indent++
	for i, it := range fd.Body.Items {
		f.leading(fieldBodyItemOffset(it, next))
		f.bodyItem(it)
		// A trailing comment belongs to this item's line, not to the whole body.
		end := next
		if i+1 < len(fd.Body.Items) {
			end = fieldBodyItemOffset(fd.Body.Items[i+1], next)
		}
		f.commit(end)
	}
	f.indent--
	f.w("};")
	f.commit(next)
}

// fieldBodyItemOffset returns a body item's source offset when it has one, else
// the fallback (used only to attach a trailing comment to the `{` line).
func fieldBodyItemOffset(it *FieldBodyItem, fallback int) int {
	switch {
	case it.Constraint != nil:
		return it.Constraint.Pos.Offset
	case it.Rewrite != nil:
		return it.Rewrite.Pos.Offset
	case it.Default != nil:
		return it.Default.Pos.Offset
	case it.OnClause != nil:
		return it.OnClause.Pos.Offset
	}
	return fallback
}

func (f *aslFmt) bodyItem(it *FieldBodyItem) {
	switch {
	case it.Constraint != nil:
		c := it.Constraint
		f.wf("constraint %s", c.Name)
		if len(c.Args) > 0 {
			f.wf("(%s)", strings.Join(c.Args, ", "))
		}
		f.w(";")
	case it.Rewrite != nil:
		r := it.Rewrite
		f.wf("rewrite %s := %s;", strings.Join(r.Events, ", "), rewriteValue(r))
	case it.Default != nil:
		f.wf("default := %s;", defaultValue(it.Default))
	case it.OnClause != nil:
		f.wf("on %s;", it.OnClause.Field)
	}
}

func (f *aslFmt) trigger(t *TriggerDecl, next int) {
	f.wf("trigger %s %s %s", t.Name, t.Timing, strings.Join(t.Events, ", "))
	if t.ForEach != "" {
		f.wf(" for each %s", t.ForEach)
	}
	if t.When != nil {
		f.wf(" when (%s)", *t.When)
	}
	if t.Do != nil {
		f.wf(" do ( %s )", strings.TrimSpace(t.Do.Raw))
	} else if t.ExecFn != nil {
		f.wf(" execute %s()", *t.ExecFn)
	}
	f.w(";")
	f.commit(next)
}

func (f *aslFmt) policy(p *PolicyDecl, next int) {
	f.wf("policy %s for %s", p.Name, strings.Join(p.Commands, ", "))
	if len(p.Roles) > 0 {
		f.wf(" to %s", strings.Join(p.Roles, ", "))
	}
	if p.Using != nil {
		f.wf(" using ( %s )", p.Using.AQL())
	}
	if p.Check != nil {
		f.wf(" with check ( %s )", p.Check.AQL())
	}
	f.w(";")
	f.commit(next)
}

func rewriteValue(r *RewriteDecl) string {
	switch {
	case r.Call != nil:
		args := make([]string, len(r.Call.Args))
		for i, a := range r.Call.Args {
			if a.Row != nil && a.Field != nil {
				args[i] = *a.Row + "." + *a.Field
			} else if a.Lit != nil {
				args[i] = *a.Lit
			}
		}
		return fmt.Sprintf("%s(%s)", r.Call.Func, strings.Join(args, ", "))
	case r.Row != nil && r.Field != nil:
		return *r.Row + "." + *r.Field
	case r.Lit != nil:
		return *r.Lit
	}
	return ""
}

func defaultValue(d *DefaultDecl) string {
	switch {
	case d.NewFunc != nil:
		return *d.NewFunc + "()"
	case len(d.QualEnum) == 2:
		return d.QualEnum[0] + "." + d.QualEnum[1]
	case d.NewLit != nil:
		return *d.NewLit
	case d.OldFunc != nil:
		return *d.OldFunc + "()"
	case d.OldLit != nil:
		return *d.OldLit
	}
	return ""
}

// dottedFields renders a list of field names as `.a, .b`.
func dottedFields(fields []string) string {
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = "." + f
	}
	return strings.Join(parts, ", ")
}

// joinTokens glues a token list (Ident / '.' / '??' / String / Int) back into
// text, suppressing spaces around '.' so `.name ?? .email` renders correctly.
func joinTokens(toks []string) string {
	var b strings.Builder
	for i, t := range toks {
		if i > 0 && t != "." && toks[i-1] != "." {
			b.WriteByte(' ')
		}
		b.WriteString(t)
	}
	return b.String()
}

// collectComments lexes src and returns its `#` comments in source order.
func collectComments(src string) []comment {
	lex, err := aslLexer.Lex("", strings.NewReader(src))
	if err != nil {
		return nil
	}
	toks, err := lexer.ConsumeAll(lex)
	if err != nil {
		return nil
	}
	ct, ok := aslLexer.Symbols()["Comment"]
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
			Line:   t.Pos.Line,
			Own:    ownLine(src, t.Pos.Offset),
		})
	}
	return out
}

// ownLine reports whether only whitespace precedes offset on its source line.
func ownLine(src string, offset int) bool {
	for i := offset - 1; i >= 0 && src[i] != '\n'; i-- {
		if src[i] != ' ' && src[i] != '\t' && src[i] != '\r' {
			return false
		}
	}
	return true
}
