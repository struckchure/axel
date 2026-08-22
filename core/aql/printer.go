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
	}
}

func printVar(b *strings.Builder, v *VarBlock) {
	if v == nil || len(v.Params) == 0 {
		return
	}
	if len(v.Params) == 1 && v.Pos.Line == v.EndPos.Line {
		b.WriteString("var ")
		printParam(b, v.Params[0])
		b.WriteString(";\n")
		return
	}
	b.WriteString("var (\n")
	for _, p := range v.Params {
		b.WriteString("  ")
		printParam(b, p)
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

func printParam(b *strings.Builder, p *Param) {
	if p == nil {
		return
	}
	fmt.Fprintf(b, "$%s", p.Name)
	if p.Type != "" {
		fmt.Fprintf(b, "<%s>", p.Type)
	}
	if p.Optional {
		b.WriteString("?")
	}
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
		fmt.Fprintf(b, "  %s := ", a.Field)
		printExpr(b, a.Value)
		if i < len(s.Assignments)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}")
	printConflict(b, s.Conflict)
	b.WriteString(";")
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
			fmt.Fprintf(b, "  %s := ", a.Field)
			printExpr(b, a.Value)
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
		fmt.Fprintf(b, "  %s := ", a.Field)
		printExpr(b, a.Value)
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
	printPrimary(b, c.Left)
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
		printPrimary(b, c.Right)
	}
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
