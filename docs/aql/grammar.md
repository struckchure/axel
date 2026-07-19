---
title: Grammar reference — AQL
description: The full AQL grammar
---

# Grammar reference

The semicolon at the end of each statement is optional when the query is passed as an inline string.

```
Statement   = SelectStmt | InsertStmt | UpdateStmt | DeleteStmt

SelectStmt  = "select" SelectBody ";"?
SelectBody  = AggExpr
            | TypeName Shape? Filter? OrderBy? Limit? Offset?

AggExpr     = Ident "(" TypeName Filter? ")"

InsertStmt  = "insert" TypeName "{" Assignment ("," Assignment)* ","? "}" Conflict? ";"?
UpdateStmt  = "update" TypeName Filter? "set" "{" Assignment ("," Assignment)* ","? "}" ";"?
DeleteStmt  = "delete" TypeName Filter? ";"?

Conflict    = "unless" "conflict" ("on" ConflictTarget)? ("else" "(" ConflictUpdate ")")?
ConflictTarget = "." Ident | "(" "." Ident ("," "." Ident)* ")"
ConflictUpdate = "update" TypeName "set" "{" Assignment ("," Assignment)* ","? "}"

Shape       = "{" ShapeField ("," ShapeField)* ","? "}"
ShapeField  = Ident (":" Shape)?          # leaf or nested link shape
            | Ident ":=" Expr             # computed field

Assignment  = Ident ":=" Expr

Filter      = "filter" Expr
OrderBy     = "order" "by" OrderItem ("," OrderItem)*
OrderItem   = Expr ("asc" | "desc")?
Limit       = "limit" Expr
Offset      = "offset" Expr

Expr        = AndExpr ("or" AndExpr)*       # `and` binds tighter than `or`
AndExpr     = Cmp ("and" Cmp)*
Cmp         = Primary (BinOp Primary)?
BinOp       = "=" | "!=" | "<" | "<=" | ">" | ">=" | "??" | "in" | "like" | "ilike"

Primary     = Operand ("<" Ident ">")?         # optional trailing cast on any operand
Operand     = "(" "multi"? "select" SelectBody ")" ("." Ident)?  # sub-select (multi → array, else single); optional field projection
            | "(" "insert" TypeName "{" ... ")" # sub-insert returning id
            | "(" Expr ")"
            | FuncCall
            | PathExpr
            | QualifiedIdent               # TypeName.field — outer-query reference
            | "$" Ident                    # named parameter
            | "null" | "true" | "false"
            | String | Int | Float | Ident

FuncCall      = Ident "(" (Expr ("," Expr)*)? ")"
PathExpr      = ("." Ident)+
QualifiedIdent = Ident "." Ident           # e.g. User.id in a sub-select filter
```
