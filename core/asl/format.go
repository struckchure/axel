package asl

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/struckchure/axel/core/aql"
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

func (f *aslFmt) hasLeadingComment(offset int) bool {
	return f.ci < len(f.cmts) && f.cmts[f.ci].Offset < offset
}

func (f *aslFmt) file(sf *SourceFile) {
	for i, d := range sf.Definitions {
		if i > 0 {
			prev := sf.Definitions[i-1]
			prevKind := defKind(prev)
			currKind := defKind(d)
			shouldGroup := prevKind == currKind && isSingleLineKind(currKind) && !f.hasLeadingComment(defOffset(d))
			if !shouldGroup {
				f.blank()
			}
		}
		f.leading(defOffset(d))
		next := 1 << 30
		if i+1 < len(sf.Definitions) {
			next = defOffset(sf.Definitions[i+1])
		}
		f.definition(d, next)
	}
}

const maxLineWidth = 80

func defKind(d *Definition) string {
	switch {
	case d.Extension != nil:
		return "extension"
	case d.Global != nil:
		return "global"
	case d.EnumType != nil:
		single := fmt.Sprintf("enum %s { %s }", d.EnumType.Name, strings.Join(d.EnumType.Values, ", "))
		if len(single) <= maxLineWidth {
			return "enum"
		}
		return "enum_multi"
	case d.ScalarType != nil:
		s := d.ScalarType
		if s.Body == nil || (len(s.Body.Fields) == 0 && len(s.Body.Items) == 0) {
			return "scalar_single"
		}
		return "scalar_multi"
	case d.Function != nil:
		return "function"
	case d.TypeDef != nil:
		return "type"
	default:
		return "other"
	}
}

func isSingleLineKind(k string) bool {
	switch k {
	case "extension", "global", "enum", "scalar_single":
		return true
	default:
		return false
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
		f.enumTypeDef(d.EnumType, next)
	case d.Function != nil:
		f.function(d.Function, next)
	case d.TypeDef != nil:
		f.typeDef(d.TypeDef, next)
	}
}

func (f *aslFmt) enumTypeDef(e *EnumTypeDef, next int) {
	single := fmt.Sprintf("enum %s { %s }", e.Name, strings.Join(e.Values, ", "))
	if len(single) <= maxLineWidth {
		f.w(single)
		f.commit(next)
		return
	}
	f.wf("enum %s {", e.Name)
	f.commit(next)
	f.indent++
	for _, val := range e.Values {
		f.wf("%s,", val)
		f.commit(next)
	}
	f.indent--
	f.w("}")
	f.commit(next)
}

func (f *aslFmt) scalarTypeDef(s *ScalarTypeDef, next int) {
	extendsStr := s.Extends
	if s.ExtendsSQL != nil {
		extendsStr = fmt.Sprintf("sql %s", *s.ExtendsSQL)
	}
	asStr := ""
	if s.AsMulti {
		asStr = fmt.Sprintf(" as multi %s", s.AsBase)
	} else if s.AsBase != "" {
		asStr = fmt.Sprintf(" as %s", s.AsBase)
	} else if s.AsBody != nil {
		asStr = " as"
	}

	body := s.Body
	if s.AsBody != nil {
		body = s.AsBody
	}

	if body == nil || (len(body.Fields) == 0 && len(body.Items) == 0) {
		f.wf("scalar type %s extends %s%s;", s.Name, extendsStr, asStr)
		f.commit(next)
		return
	}
	f.wf("scalar type %s extends %s%s {", s.Name, extendsStr, asStr)
	if len(body.Items) > 0 {
		f.commit(fieldBodyItemOffset(body.Items[0], next))
	} else if len(body.Fields) > 0 {
		f.commit(body.Fields[0].Pos.Offset)
	} else {
		f.commit(next)
	}
	f.indent++
	for i, it := range body.Items {
		f.leading(fieldBodyItemOffset(it, next))
		f.bodyItem(it)
		end := next
		if i+1 < len(body.Items) {
			end = fieldBodyItemOffset(body.Items[i+1], next)
		} else if len(body.Fields) > 0 {
			end = body.Fields[0].Pos.Offset
		}
		f.commit(end)
	}
	for i, field := range body.Fields {
		f.leading(field.Pos.Offset)
		fieldNext := next
		if i+1 < len(body.Fields) {
			fieldNext = body.Fields[i+1].Pos.Offset
		}
		req := ""
		if field.Required {
			req = "required "
		}
		multi := ""
		if field.Multi {
			multi = "multi "
		}
		if field.Computed != nil {
			f.wf("%s%s%s: %s := %s;", req, multi, field.Name, field.Type, field.Computed.Raw)
		} else {
			f.wf("%s%s%s: %s;", req, multi, field.Name, field.Type)
		}
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
	singleHeader := fmt.Sprintf("function %s(%s) -> %s {", fn.Name, strings.Join(params, ", "), ret)
	if len(singleHeader) <= maxLineWidth || len(params) <= 1 {
		f.w(singleHeader)
	} else {
		f.wf("function %s(", fn.Name)
		f.commit(next)
		f.indent++
		for i, p := range params {
			comma := ","
			if i == len(params)-1 {
				comma = ""
			}
			f.wf("%s%s", p, comma)
			f.commit(next)
		}
		f.indent--
		f.wf(") -> %s {", ret)
	}
	if fn.Return != nil {
		f.commit(fn.Return.Pos.Offset)
		lines := formatSQLExprLines(fn.Return.Raw, f.indent+1)
		for _, line := range lines {
			content := line
			if strings.HasPrefix(content, strings.Repeat("  ", f.indent)) {
				content = content[len(strings.Repeat("  ", f.indent)):]
			}
			f.w(content)
			f.commit(fn.EndPos.Offset)
		}
		f.w("};")
	} else {
		f.w("};")
	}
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
	header := fmt.Sprintf("trigger %s %s %s", t.Name, t.Timing, strings.Join(t.Events, ", "))
	if t.ForEach != "" {
		header += " for each " + t.ForEach
	}
	if t.When != nil {
		header += fmt.Sprintf(" when (%s)", *t.When)
	}

	if t.ExecFn != nil {
		f.wf("%s execute %s();", header, *t.ExecFn)
		f.commit(next)
		return
	}

	if t.Do != nil {
		raw := strings.TrimSpace(t.Do.Raw)
		stmtSrc := strings.TrimSuffix(raw, ";")
		formatted := stmtSrc
		if aqlOut, err := aql.Format([]byte(stmtSrc)); err == nil {
			formatted = strings.TrimSpace(aqlOut)
			formatted = strings.TrimSuffix(formatted, ";")
		}

		lines := strings.Split(formatted, "\n")
		indentLen := f.indent * 2
		if len(lines) == 1 && indentLen+len(header)+len(lines[0])+10 <= maxLineWidth {
			f.wf("%s do ( %s );", header, lines[0])
			f.commit(next)
			return
		}

		f.wf("%s do (", header)
		f.commit(next)
		f.indent++
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			leadingSpaces := len(line) - len(strings.TrimLeft(line, " "))
			extraIndent := leadingSpaces / 2
			f.w(strings.Repeat("  ", extraIndent) + trimmed)
			f.commit(next)
		}
		f.indent--
		f.w(");")
		f.commit(next)
	}
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
	case d.NewCall != nil:
		if len(d.NewCall.Args) == 0 {
			return d.NewCall.Func + "()"
		}
		args := make([]string, len(d.NewCall.Args))
		for i, a := range d.NewCall.Args {
			args[i] = *a.Lit
		}
		return fmt.Sprintf("%s(%s)", d.NewCall.Func, strings.Join(args, ", "))
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

// ─────────────────────────────────────────────────────────────
// SQL Return Expression Formatting
// ─────────────────────────────────────────────────────────────

type sqlExprNode struct {
	kind     string // "token", "call", "group"
	name     string
	tok      lexer.Token
	children []*sqlExprNode
}

func parseSQLExprNodes(toks []lexer.Token) []*sqlExprNode {
	var nodes []*sqlExprNode
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Value == "(" {
			var callNode *sqlExprNode
			if len(nodes) > 0 && nodes[len(nodes)-1].kind == "token" && nodes[len(nodes)-1].tok.Type == aslLexer.Symbols()["Ident"] {
				last := nodes[len(nodes)-1]
				nodes = nodes[:len(nodes)-1]
				callNode = &sqlExprNode{kind: "call", name: last.tok.Value}
			}

			depth := 1
			start := i + 1
			j := start
			for ; j < len(toks); j++ {
				if toks[j].Value == "(" {
					depth++
				} else if toks[j].Value == ")" {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			var innerToks []lexer.Token
			if j > start {
				innerToks = toks[start:j]
			}
			children := parseSQLExprNodes(innerToks)

			if callNode != nil {
				callNode.children = children
				nodes = append(nodes, callNode)
			} else {
				groupNode := &sqlExprNode{kind: "group", children: children}
				nodes = append(nodes, groupNode)
			}
			i = j
			continue
		}
		nodes = append(nodes, &sqlExprNode{kind: "token", tok: t})
	}
	return nodes
}

func isBinaryOp(s string) bool {
	switch s {
	case "+", "-", "*", "/", "^", "%", "=", "!=", "<", "<=", ">", ">=", "||", "AND", "OR", "and", "or", "in", "like", "ilike":
		return true
	}
	return false
}

func needsSpace(prev, next *sqlExprNode) bool {
	if prev == nil || next == nil {
		return false
	}
	// Inline AQL: aql`...`
	if prev.kind == "token" && prev.tok.Value == "aql" && next.kind == "token" && next.tok.Type == aslLexer.Symbols()["AQLString"] {
		return false
	}
	// Comma
	if prev.kind == "token" && prev.tok.Value == "," {
		return true
	}
	if next.kind == "token" && (next.tok.Value == "," || next.tok.Value == ".") {
		return false
	}
	if prev.kind == "token" && prev.tok.Value == "." {
		return false
	}
	// Type cast ::
	if next.kind == "token" && next.tok.Value == ":" {
		return false
	}
	if prev.kind == "token" && prev.tok.Value == ":" {
		return false
	}
	// Multi-char operators: ||, !=, <=, >=, <>
	if prev.kind == "token" && next.kind == "token" {
		if prev.tok.Value == "|" && next.tok.Value == "|" {
			return false
		}
		if (prev.tok.Value == "!" || prev.tok.Value == "<" || prev.tok.Value == ">") && next.tok.Value == "=" {
			return false
		}
		if prev.tok.Value == "<" && next.tok.Value == ">" {
			return false
		}
	}
	// String concatenation ||
	if prev.kind == "token" && prev.tok.Value == "|" {
		return true
	}
	if next.kind == "token" && next.tok.Value == "|" {
		return true
	}
	// Binary operators
	if prev.kind == "token" && isBinaryOp(prev.tok.Value) {
		return true
	}
	if next.kind == "token" && isBinaryOp(next.tok.Value) {
		return true
	}
	if prev.kind == "token" && (prev.tok.Type == aslLexer.Symbols()["Ident"] || prev.tok.Type == aslLexer.Symbols()["Int"] || prev.tok.Type == aslLexer.Symbols()["String"] || prev.tok.Type == aslLexer.Symbols()["AQLString"]) {
		if next.kind == "token" || next.kind == "call" || next.kind == "group" {
			return true
		}
	}
	if prev.kind == "call" || prev.kind == "group" {
		if next.kind == "token" || next.kind == "call" || next.kind == "group" {
			return true
		}
	}
	return false
}

func renderSingle(nodes []*sqlExprNode) string {
	var sb strings.Builder
	for i, n := range nodes {
		if i > 0 {
			if needsSpace(nodes[i-1], n) {
				sb.WriteByte(' ')
			}
		}
		switch n.kind {
		case "token":
			sb.WriteString(n.tok.Value)
		case "call":
			sb.WriteString(n.name)
			sb.WriteByte('(')
			sb.WriteString(renderSingle(n.children))
			sb.WriteByte(')')
		case "group":
			sb.WriteByte('(')
			sb.WriteString(renderSingle(n.children))
			sb.WriteByte(')')
		}
	}
	return sb.String()
}

func formatSQLExprLines(raw string, indent int) []string {
	l, err := aslLexer.Lex("", strings.NewReader(raw))
	if err != nil {
		return []string{strings.Repeat("  ", indent) + "return " + raw + ";"}
	}
	var toks []lexer.Token
	for {
		t, err := l.Next()
		if err != nil || t.EOF() {
			break
		}
		if t.Type != aslLexer.Symbols()["Whitespace"] && t.Type != aslLexer.Symbols()["Comment"] {
			toks = append(toks, t)
		}
	}
	if len(toks) == 0 {
		return []string{strings.Repeat("  ", indent) + "return ;"}
	}
	nodes := parseSQLExprNodes(toks)
	return formatExprLines("return ", nodes, ";", indent)
}

func formatExprLines(prefix string, nodes []*sqlExprNode, suffix string, indent int) []string {
	single := prefix + renderSingle(nodes) + suffix
	if len(strings.Repeat("  ", indent)+single) <= maxLineWidth {
		return []string{strings.Repeat("  ", indent) + single}
	}

	// Check if nodes contain a trailing call/group that can be expanded
	if len(nodes) > 0 && (nodes[len(nodes)-1].kind == "call" || nodes[len(nodes)-1].kind == "group") {
		last := nodes[len(nodes)-1]
		leadNodes := nodes[:len(nodes)-1]
		leadStr := renderSingle(leadNodes)
		if len(leadStr) > 0 && needsSpace(leadNodes[len(leadNodes)-1], last) {
			leadStr += " "
		}

		var openStr, closeStr string
		if last.kind == "call" {
			openStr = leadStr + last.name + "("
		} else {
			openStr = leadStr + "("
		}
		closeStr = ")" + suffix

		// Check if children have commas (arguments)
		args := splitByComma(last.children)
		if len(args) > 1 {
			var lines []string
			lines = append(lines, strings.Repeat("  ", indent)+prefix+openStr)
			for idx, arg := range args {
				argSuffix := ","
				if idx == len(args)-1 {
					argSuffix = ""
				}
				argLines := formatExprLines("", arg, argSuffix, indent+1)
				lines = append(lines, argLines...)
			}
			lines = append(lines, strings.Repeat("  ", indent)+closeStr)
			return lines
		}

		// Children is a single expression inside parens (like sqrt(...) or arithmetic)
		terms := splitByBinaryOp(last.children)
		if len(terms) > 1 {
			var lines []string
			lines = append(lines, strings.Repeat("  ", indent)+prefix+openStr)
			for _, term := range terms {
				subTerms := splitByBinaryOp(term.nodes)
				if len(subTerms) > 1 {
					subPacked := packTerms(subTerms, indent+1)
					if len(subPacked) > 0 && term.op != "" {
						subPacked[len(subPacked)-1] += term.op
					}
					lines = append(lines, subPacked...)
				} else {
					tStr := renderSingle(term.nodes)
					if term.op != "" {
						tStr += term.op
					}
					lines = append(lines, strings.Repeat("  ", indent+1)+tStr)
				}
			}
			lines = append(lines, strings.Repeat("  ", indent)+closeStr)
			return lines
		}

		// Single child call/group inside
		if len(last.children) == 1 && (last.children[0].kind == "call" || last.children[0].kind == "group") {
			var lines []string
			lines = append(lines, strings.Repeat("  ", indent)+prefix+openStr)
			innerLines := formatExprLines("", last.children, "", indent+1)
			lines = append(lines, innerLines...)
			lines = append(lines, strings.Repeat("  ", indent)+closeStr)
			return lines
		}
	}

	// Fallback: split top-level by binary ops
	terms := splitByBinaryOp(nodes)
	if len(terms) > 1 {
		return packTerms(terms, indent)
	}

	return []string{strings.Repeat("  ", indent) + single}
}

type sqlTerm struct {
	nodes []*sqlExprNode
	op    string
}

func packTerms(terms []sqlTerm, indent int) []string {
	var lines []string
	curLine := ""
	for _, t := range terms {
		tStr := renderSingle(t.nodes)
		if t.op != "" {
			tStr += t.op
		}
		if curLine == "" {
			curLine = tStr
		} else {
			if len(strings.Repeat("  ", indent)+curLine+" "+tStr) <= maxLineWidth {
				curLine += " " + tStr
			} else {
				lines = append(lines, strings.Repeat("  ", indent)+curLine)
				curLine = tStr
			}
		}
	}
	if curLine != "" {
		lines = append(lines, strings.Repeat("  ", indent)+curLine)
	}
	return lines
}

func splitByComma(nodes []*sqlExprNode) [][]*sqlExprNode {
	var res [][]*sqlExprNode
	var cur []*sqlExprNode
	for _, n := range nodes {
		if n.kind == "token" && n.tok.Value == "," {
			res = append(res, cur)
			cur = nil
		} else {
			cur = append(cur, n)
		}
	}
	if len(cur) > 0 || len(res) > 0 {
		res = append(res, cur)
	}
	return res
}

func splitByBinaryOp(nodes []*sqlExprNode) []sqlTerm {
	// First look for + or -
	var res []sqlTerm
	var cur []*sqlExprNode
	found := false
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		if n.kind == "token" && (n.tok.Value == "+" || n.tok.Value == "-") && len(cur) > 0 {
			res = append(res, sqlTerm{nodes: cur, op: " " + n.tok.Value})
			cur = nil
			found = true
		} else {
			cur = append(cur, n)
		}
	}
	if found && len(cur) > 0 {
		res = append(res, sqlTerm{nodes: cur, op: ""})
		return res
	}

	// Next look for * or / if any term is long
	res = nil
	cur = nil
	found = false
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		if n.kind == "token" && (n.tok.Value == "*" || n.tok.Value == "/") && len(cur) > 0 {
			res = append(res, sqlTerm{nodes: cur, op: " " + n.tok.Value})
			cur = nil
			found = true
		} else {
			cur = append(cur, n)
		}
	}
	if found && len(cur) > 0 {
		res = append(res, sqlTerm{nodes: cur, op: ""})
		return res
	}

	return nil
}

