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

func printSelect(b *strings.Builder, s *SelectStmt) {
	b.WriteString("select ")
	body := s.Body
	if body.AggFunc != nil {
		fmt.Fprintf(b, "%s(%s", body.AggFunc.Func, body.AggFunc.TypeName)
		if body.AggFunc.Filter != nil {
			b.WriteString(" ")
			printFilter(b, body.AggFunc.Filter)
		}
		b.WriteString(")")
	} else {
		b.WriteString(body.TypeName)
		if body.Shape != nil {
			b.WriteString(" ")
			printShape(b, body.Shape)
		}
		if body.Filter != nil {
			b.WriteString("\n")
			printFilter(b, body.Filter)
		}
		for i, o := range body.OrderBy {
			if i == 0 {
				b.WriteString("\norder by ")
			} else {
				b.WriteString(", ")
			}
			printExpr(b, o.Expr)
			if o.Dir != "" {
				fmt.Fprintf(b, " %s", o.Dir)
			}
		}
		if body.Limit != nil {
			b.WriteString("\nlimit ")
			printExpr(b, body.Limit)
		}
		if body.Offset != nil {
			b.WriteString("\noffset ")
			printExpr(b, body.Offset)
		}
	}
	b.WriteString(";")
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
	b.WriteString("};")
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

func printShape(b *strings.Builder, s *Shape) {
	b.WriteString("{\n")
	for i, f := range s.Fields {
		b.WriteString("  ")
		b.WriteString(f.Name)
		if f.SubShape != nil {
			b.WriteString(": ")
			printShape(b, f.SubShape)
		}
		if i < len(s.Fields)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}")
}

func printFilter(b *strings.Builder, f *Filter) {
	b.WriteString("filter ")
	printExpr(b, f.Expr)
}

func printExpr(b *strings.Builder, e *Expr) {
	if e == nil {
		return
	}
	printPrimary(b, e.Left)
	if e.Op != "" {
		fmt.Fprintf(b, " %s ", e.Op)
		printPrimary(b, e.Right)
	}
}

func printPrimary(b *strings.Builder, p *Primary) {
	if p == nil {
		return
	}
	switch {
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
	case p.Path != nil:
		b.WriteString("." + strings.Join(p.Path.Steps, "."))
	case p.Param != nil:
		fmt.Fprintf(b, "$%s", *p.Param)
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
	case p.Ident != nil:
		b.WriteString(*p.Ident)
	}
}
