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
		s := d.ScalarType
		f.wf("scalar type %s extending %s;", s.Name, s.Extends)
		f.commit(next)
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
		f.wf(" extending %s", strings.Join(t.Extending, ", "))
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
	prevLine := 0
	for i, m := range t.Members {
		mo := memberOffset(m)
		// Preserve a single author blank line between members.
		if i > 0 && prevLine > 0 && mPosLine(m)-prevLine > 1 {
			f.blank()
		}
		f.leading(mo)
		next := t.EndPos.Offset
		if i+1 < len(t.Members) {
			next = memberOffset(t.Members[i+1])
		}
		f.member(m, next)
		prevLine = mPosLine(m)
	}
	f.indent--
	f.w("}")
	f.commit(next)
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
		f.wf(" on (%s);", dottedFields(c.Fields))
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
	for _, it := range fd.Body.Items {
		f.bodyItem(it)
		f.commit(next)
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
	f.wf("policy %s for %s", p.Name, p.Command)
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
