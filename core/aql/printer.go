package aql

import (
	"fmt"
	"strings"
)

// Print returns a human-readable representation of an AQL statement.
func Print(stmt *Statement) string {
	var b strings.Builder
	printStmt(&b, stmt)
	return b.String()
}

func printStmt(b *strings.Builder, stmt *Statement) {
	for _, d := range stmt.Directives {
		fmt.Fprintf(b, "@%s %s\n", d.Name, d.Value)
	}
	if len(stmt.Directives) > 0 {
		b.WriteString("\n")
	}
	for _, v := range stmt.Vars {
		printVar(b, v)
	}
	printWith(b, stmt.With)
	switch {
	case stmt.Select != nil:
		printSelect(b, stmt.Select)
	case stmt.Insert != nil:
		printInsert(b, stmt.Insert)
	case stmt.Update != nil:
		printUpdate(b, stmt.Update)
	case stmt.Delete != nil:
		printDelete(b, stmt.Delete)
	case stmt.For != nil:
		printFor(b, stmt.For)
	}
}

func printVar(b *strings.Builder, v *VarBlock) {
	if v == nil || len(v.Params) == 0 {
		return
	}
	if len(v.Params) == 1 && v.Pos.Line == v.EndPos.Line {
		b.WriteString("var ")
		printVarParam(b, v.Params[0])
		b.WriteString(";\n")
		return
	}
	b.WriteString("var (\n")
	for _, p := range v.Params {
		b.WriteString("  ")
		printVarParam(b, p)
		b.WriteString(";\n")
	}
	b.WriteString(")\n")
}

// printWith renders a with-block, one binding per indented line. It stays
// deliberately layout-fixed (never width-aware): Print is the oracle Format
// compares against to prove a reformat was lossless, so its output must depend
// only on the AST.
func printWith(b *strings.Builder, w *WithBlock) {
	if w == nil {
		return
	}
	b.WriteString("with (\n")
	for _, bind := range w.Bindings {
		fmt.Fprintf(b, "  %s := ", bind.Name)
		printExpr(b, bind.Value)
		b.WriteString(";\n")
	}
	b.WriteString(")\n")
}

func printVarParam(b *strings.Builder, p *VarParam) {
	if p == nil {
		return
	}
	if p.Multi {
		b.WriteString("multi ")
	}
	name := p.Name
	if !strings.HasPrefix(name, "$") {
		name = "$" + name
	}
	b.WriteString(name)
	if p.ColonType != "" {
		fmt.Fprintf(b, ": %s", p.ColonType)
	} else if p.Type != "" {
		fmt.Fprintf(b, "<%s>", p.Type)
	}
	if p.Optional {
		b.WriteString("?")
	}
	if p.Default != nil {
		b.WriteString(" := ")
		if p.Default.Set != nil {
			printSetLiteral(b, p.Default.Set)
		} else if p.Default.Expr != nil {
			printExpr(b, p.Default.Expr)
		}
	}
}

func printParam(b *strings.Builder, p *Param) {
	if p == nil {
		return
	}
	name := p.Name
	if !strings.HasPrefix(name, "$") {
		name = "$" + name
	}
	b.WriteString(name)
	if p.ColonType != "" {
		fmt.Fprintf(b, ": %s", p.ColonType)
	} else if p.Type != "" {
		fmt.Fprintf(b, "<%s>", p.Type)
	}
	if p.Optional {
		b.WriteString("?")
	}
}

func printSetLiteral(b *strings.Builder, s *SetLiteral) {
	if s == nil {
		return
	}
	b.WriteString("{")
	for i, elem := range s.Elements {
		if i > 0 {
			b.WriteString(", ")
		}
		printExpr(b, elem)
	}
	b.WriteString("}")
}

func printFor(b *strings.Builder, f *ForStmt) {
	if f == nil {
		return
	}
	iter := f.Iterator
	if !strings.HasPrefix(iter, "$") {
		iter = "$" + iter
	}
	fmt.Fprintf(b, "for %s in ", iter)
	printExpr(b, f.InExpr)
	b.WriteString(" {\n")
	if f.Body != nil {
		switch {
		case f.Body.Insert != nil:
			printInsert(b, f.Body.Insert)
		case f.Body.Select != nil:
			printSelect(b, f.Body.Select)
		case f.Body.Update != nil:
			printUpdate(b, f.Body.Update)
		case f.Body.Delete != nil:
			printDelete(b, f.Body.Delete)
		}
	}
	b.WriteString("\n}")
}

func printSelect(b *strings.Builder, s *SelectStmt) {
	if s.Multi {
		b.WriteString("multi ")
	}
	b.WriteString("select ")
	printSelectBody(b, s.Body, "\n")
	b.WriteString(";")
}

// printSelectBody renders a select body (aggregate or object). sep separates the
// filter/order/limit/offset clauses — "\n" for a statement, " " when inline in a
// subquery.
func printSelectBody(b *strings.Builder, body *SelectBody, sep string) {
	if body.AggFunc != nil {
		fmt.Fprintf(b, "%s(%s", body.AggFunc.Func, body.AggFunc.TypeName)
		if body.AggFunc.Filter != nil {
			b.WriteString(" ")
			printFilter(b, body.AggFunc.Filter)
		}
		b.WriteString(")")
		return
	}
	b.WriteString(body.TypeName)
	if body.Shape != nil {
		b.WriteString(" ")
		printShape(b, body.Shape)
	}
	if body.Filter != nil {
		b.WriteString(sep)
		printFilter(b, body.Filter)
	}
	for i, g := range body.GroupBy {
		if i == 0 {
			b.WriteString(sep + "group by ")
		} else {
			b.WriteString(", ")
		}
		printExpr(b, g.Expr)
	}
	if body.Having != nil {
		b.WriteString(sep + "having ")
		printExpr(b, body.Having.Expr)
	}
	for i, o := range body.OrderBy {
		if i == 0 {
			b.WriteString(sep + "order by ")
		} else {
			b.WriteString(", ")
		}
		printExpr(b, o.Expr)
		if o.Dir != "" {
			fmt.Fprintf(b, " %s", o.Dir)
		}
	}
	if body.Limit != nil {
		b.WriteString(sep + "limit ")
		printExpr(b, body.Limit)
	}
	if body.Offset != nil {
		b.WriteString(sep + "offset ")
		printExpr(b, body.Offset)
	}
}

// printSelectBodyInline renders a select body with all clauses space-separated
// and the shape (if any) on a single line. Used by printPrimary so subqueries
// inside expressions are always compact: `(select T { id } filter .x = $y)`.
func printSelectBodyInline(b *strings.Builder, body *SelectBody) {
	if body.AggFunc != nil {
		fmt.Fprintf(b, "%s(%s", body.AggFunc.Func, body.AggFunc.TypeName)
		if body.AggFunc.Filter != nil {
			b.WriteString(" ")
			printFilter(b, body.AggFunc.Filter)
		}
		b.WriteString(")")
		return
	}
	b.WriteString(body.TypeName)
	if body.Shape != nil {
		b.WriteString(" ")
		printShapeInline(b, body.Shape)
	}
	if body.Filter != nil {
		b.WriteString(" ")
		printFilter(b, body.Filter)
	}
	for i, g := range body.GroupBy {
		if i == 0 {
			b.WriteString(" group by ")
		} else {
			b.WriteString(", ")
		}
		printExpr(b, g.Expr)
	}
	if body.Having != nil {
		b.WriteString(" having ")
		printExpr(b, body.Having.Expr)
	}
	for i, o := range body.OrderBy {
		if i == 0 {
			b.WriteString(" order by ")
		} else {
			b.WriteString(", ")
		}
		printExpr(b, o.Expr)
		if o.Dir != "" {
			fmt.Fprintf(b, " %s", o.Dir)
		}
	}
	if body.Limit != nil {
		b.WriteString(" limit ")
		printExpr(b, body.Limit)
	}
	if body.Offset != nil {
		b.WriteString(" offset ")
		printExpr(b, body.Offset)
	}
}


func printInsert(b *strings.Builder, s *InsertStmt) {
	fmt.Fprintf(b, "insert %s {\n", s.TypeName)
	for i, a := range s.Assignments {
		printAssignment(b, a)
		if i < len(s.Assignments)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}")
	printConflict(b, s.Conflict)
	b.WriteString(";")
}

func printAssignment(b *strings.Builder, a *Assignment) {
	fmt.Fprintf(b, "  %s := ", a.Field)
	if a.LinkDelta != nil {
		printLinkDelta(b, a.LinkDelta)
	} else {
		printExpr(b, a.Value)
	}
}

func printLinkDelta(b *strings.Builder, d *LinkDelta) {
	b.WriteString("{\n")
	for i, item := range d.Items {
		op := item.NormalizedOp()
		fmt.Fprintf(b, "    %q: ", op)
		printExpr(b, item.Value)
		if i < len(d.Items)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  }")
}

func printConflict(b *strings.Builder, c *OnConflict) {
	if c == nil {
		return
	}
	b.WriteString(" unless conflict")
	if c.Target != nil {
		switch len(c.Target.Fields) {
		case 0:
		case 1:
			fmt.Fprintf(b, " on .%s", c.Target.Fields[0])
		default:
			b.WriteString(" on (")
			for i, f := range c.Target.Fields {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(b, ".%s", f)
			}
			b.WriteString(")")
		}
	}
	if c.Else != nil {
		fmt.Fprintf(b, " else (update %s set {\n", c.Else.TypeName)
		for i, a := range c.Else.Assignments {
			printAssignment(b, a)
			if i < len(c.Else.Assignments)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString("})")
	}
}

func printUpdate(b *strings.Builder, s *UpdateStmt) {
	fmt.Fprintf(b, "update %s", s.TypeName)
	if s.Filter != nil {
		b.WriteString(" ")
		printFilter(b, s.Filter)
	}
	b.WriteString("\nset {\n")
	for i, a := range s.Assignments {
		printAssignment(b, a)
		if i < len(s.Assignments)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("};")
}

func printDelete(b *strings.Builder, s *DeleteStmt) {
	fmt.Fprintf(b, "delete %s", s.TypeName)
	if s.Filter != nil {
		b.WriteString(" ")
		printFilter(b, s.Filter)
	}
	b.WriteString(";")
}

// printShape renders a shape in canonical multi-line form (used by Print and
// the re-parse oracle). For inline use inside subquery expressions, call
// printShapeInline instead.
func printShape(b *strings.Builder, s *Shape) {
	b.WriteString("{\n")
	for i, f := range s.Fields {
		b.WriteString("  ")
		if f.Star {
			b.WriteString("*")
		} else {
			b.WriteString(f.Name)
		}
		if f.SubShape != nil {
			b.WriteString(": ")
			printShape(b, f.SubShape)
		}
		if f.Computed != nil {
			b.WriteString(" := ")
			printExpr(b, f.Computed)
		}
		if f.AggFilter != nil {
			b.WriteString(" ")
			printFilter(b, f.AggFilter)
		}
		if i < len(s.Fields)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}")
}

// printShapeInline renders a shape on a single line: `{ id, name, email }`.
// Used by printPrimary so subquery shapes inside expressions stay compact.
func printShapeInline(b *strings.Builder, s *Shape) {
	b.WriteString("{ ")
	for i, f := range s.Fields {
		if i > 0 {
			b.WriteString(", ")
		}
		if f.Star {
			b.WriteString("*")
		} else {
			b.WriteString(f.Name)
		}
		if f.SubShape != nil {
			b.WriteString(": ")
			printShapeInline(b, f.SubShape)
		}
		if f.Computed != nil {
			b.WriteString(" := ")
			printExpr(b, f.Computed)
		}
		if f.AggFilter != nil {
			b.WriteString(" ")
			printFilter(b, f.AggFilter)
		}
	}
	b.WriteString(" }")
}

func printFilter(b *strings.Builder, f *Filter) {
	b.WriteString("filter ")
	printExpr(b, f.Expr)
}

func printExpr(b *strings.Builder, e *Expr) {
	if e == nil {
		return
	}
	printAndExpr(b, e.Left)
	for _, a := range e.Rest {
		b.WriteString(" or ")
		printAndExpr(b, a)
	}
}

func printAndExpr(b *strings.Builder, a *AndExpr) {
	if a == nil {
		return
	}
	printCmp(b, a.Left)
	for _, c := range a.Rest {
		b.WriteString(" and ")
		printCmp(b, c)
	}
}

func printCmp(b *strings.Builder, c *Cmp) {
	if c == nil {
		return
	}
	printAddExpr(b, c.Left)
	if c.Is {
		if c.IsNot {
			b.WriteString(" is not null")
		} else {
			b.WriteString(" is null")
		}
		return
	}
	if c.Op != "" {
		fmt.Fprintf(b, " %s ", c.Op)
		printAddExpr(b, c.Right)
	}
}

func printAddExpr(b *strings.Builder, a *AddExpr) {
	if a == nil {
		return
	}
	printMulExpr(b, a.Left)
	for _, op := range a.Rest {
		fmt.Fprintf(b, " %s ", op.Op)
		printMulExpr(b, op.Right)
	}
}

func printMulExpr(b *strings.Builder, m *MulExpr) {
	if m == nil {
		return
	}
	printFactor(b, m.Left)
	for _, op := range m.Rest {
		fmt.Fprintf(b, " %s ", op.Op)
		printFactor(b, op.Right)
	}
}

func printFactor(b *strings.Builder, f *Factor) {
	if f == nil {
		return
	}
	if f.Unary != nil {
		b.WriteString(*f.Unary)
	}
	printPrimary(b, f.Primary)
}

func printPrimary(b *strings.Builder, p *Primary) {
	if p == nil {
		return
	}
	switch {
	case p.SubQuery != nil:
		b.WriteString("(")
		if p.SubQueryMulti {
			b.WriteString("multi ")
		}
		b.WriteString("select ")
		printSelectBodyInline(b, p.SubQuery)
		b.WriteString(")")
		if p.SubQueryField != "" {
			b.WriteString("." + p.SubQueryField)
		}
	case p.SubExpr != nil:
		b.WriteString("(")
		printExpr(b, p.SubExpr)
		b.WriteString(")")
	case p.Set != nil:
		printSetLiteral(b, p.Set)
	case p.FuncCall != nil:
		fmt.Fprintf(b, "%s(", p.FuncCall.Name)
		for i, a := range p.FuncCall.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			printExpr(b, a)
		}
		b.WriteString(")")
	case p.QualifiedIdent != nil:
		b.WriteString(p.QualifiedIdent.TypeName + "." + p.QualifiedIdent.Field)
		for _, fld := range p.QualifiedIdent.Fields {
			b.WriteString("." + fld)
		}
	case p.Path != nil:
		b.WriteString("." + strings.Join(p.Path.Steps, "."))
	case p.Param != nil:
		fmt.Fprintf(b, "$%s", p.Param.Name)
		if p.Param.Type != "" {
			fmt.Fprintf(b, "<%s>", p.Param.Type)
		}
		if p.Param.Optional {
			b.WriteString("?")
		}
	case p.Null:
		b.WriteString("null")
	case p.True:
		b.WriteString("true")
	case p.False:
		b.WriteString("false")
	case p.Str != nil:
		b.WriteString(*p.Str)
	case p.Int != nil:
		b.WriteString(*p.Int)
	case p.Float != nil:
		b.WriteString(*p.Float)
	case p.GlobalRef != nil:
		b.WriteString("global " + *p.GlobalRef)
	case p.Ident != nil:
		b.WriteString(*p.Ident)
	}
	if p.Cast != "" {
		b.WriteString("<" + p.Cast + ">")
	}
}
