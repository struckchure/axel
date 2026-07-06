package aql

// Statement is the top-level AQL query node.
// Optional directives may precede the statement; exactly one of the statement
// fields will be non-nil per parsed statement.
type Statement struct {
	Directives []*Directive `parser:"@@*"`
	Select     *SelectStmt  `parser:"( @@"`
	Insert     *InsertStmt  `parser:"| @@"`
	Update     *UpdateStmt  `parser:"| @@"`
	Delete     *DeleteStmt  `parser:"| @@ )"`
}

// Directive is a leading `@name value` declaration that carries codegen metadata
// (e.g. @response User, @request CreateUserInput, @name CreateUser). Unknown
// directives are parsed and preserved but ignored by the compiler.
type Directive struct {
	Name  string `parser:"'@' @Ident"`
	Value string `parser:"@( Ident | String | Int )"`
}

// DirectiveMap returns the statement's directives as a name→value map
// (first occurrence wins). String directive values keep their surrounding quotes.
func (s *Statement) DirectiveMap() map[string]string {
	if len(s.Directives) == 0 {
		return nil
	}
	m := make(map[string]string, len(s.Directives))
	for _, d := range s.Directives {
		if _, exists := m[d.Name]; !exists {
			m[d.Name] = d.Value
		}
	}
	return m
}

// SelectStmt handles both regular selects and aggregate selects.
//
//	select User { id, email } filter .active = true order by .created_at desc limit $n;
//	select count(User filter .active = true);
type SelectStmt struct {
	Multi bool        `parser:"@'multi'? 'select'"`
	Body  *SelectBody `parser:"@@"`
	End   string      `parser:"';'?"`
}

// SelectBody holds the select content — either an aggregate or a typed shape query.
// Wrapping in a sub-struct ensures the ";" terminator in SelectStmt is always consumed
// regardless of which alternative wins inside SelectBody.
type SelectBody struct {
	// Aggregate: count(TypeName filter expr)
	AggFunc  *AggExpr `parser:"  @@"`
	// Object: TypeName { shape } filter ... order by ... limit ... offset ...
	TypeName string   `parser:"| @Ident"`
	Shape    *Shape   `parser:"@@?"`
	Filter   *Filter  `parser:"@@?"`
	OrderBy  []*Order `parser:"( 'order' 'by' @@ ( ',' @@ )* )?"`
	Limit    *Expr    `parser:"( 'limit' @@ )?"`
	Offset   *Expr    `parser:"( 'offset' @@ )?"`
}

// AggExpr: count(TypeName filter expr)
type AggExpr struct {
	Func     string  `parser:"@Ident '('"`
	TypeName string  `parser:"@Ident"`
	Filter   *Filter `parser:"@@?"`
	End      string  `parser:"')'"`
}

// InsertStmt: insert TypeName { field := expr, ... };
type InsertStmt struct {
	TypeName    string        `parser:"'insert' @Ident"`
	Assignments []*Assignment `parser:"'{' @@ ( ',' @@ )* ','? '}'"`
	End         string        `parser:"';'?"`
}

// InsertBody is a bare insert without a trailing ';', used as a sub-expression.
//
//	(insert User { email := $email })
type InsertBody struct {
	TypeName    string        `parser:"'insert' @Ident"`
	Assignments []*Assignment `parser:"'{' @@ ( ',' @@ )* ','? '}'"`
}

// UpdateStmt: update TypeName filter expr set { field := expr, ... };
type UpdateStmt struct {
	TypeName    string        `parser:"'update' @Ident"`
	Filter      *Filter       `parser:"@@?"`
	Assignments []*Assignment `parser:"'set' '{' @@ ( ',' @@ )* ','? '}'"`
	End         string        `parser:"';'?"`
}

// DeleteStmt: delete TypeName filter expr;
type DeleteStmt struct {
	TypeName string  `parser:"'delete' @Ident"`
	Filter   *Filter `parser:"@@?"`
	End      string  `parser:"';'?"`
}

// Shape is a set of selected fields, possibly with nested shapes.
//
//	{ id, email, posts: { title } }
type Shape struct {
	Fields []*ShapeField `parser:"'{' @@ ( ',' @@ )* ','? '}'"`
}

// ShapeField is one entry in a shape.
//
//	id               → leaf field
//	posts: { title } → nested link with sub-shape
//	posts := (...)   → inline computed field
type ShapeField struct {
	Name     string `parser:"@Ident"`
	SubShape *Shape `parser:"( ':' @@ )?"`
	Computed *Expr  `parser:"( ':=' @@ )?"`
}

// QualifiedIdent is a TypeName.field reference used in expressions (e.g. User.id).
type QualifiedIdent struct {
	TypeName string `parser:"@Ident '.'"`
	Field    string `parser:"@Ident"`
}

// Assignment is a field value assignment used in INSERT and UPDATE.
//
//	email := $email
type Assignment struct {
	Field string `parser:"@Ident ':='"`
	Value *Expr  `parser:"@@"`
}

// Filter is a WHERE clause.
//
//	filter .active = true and .age >= $min_age
type Filter struct {
	Expr *Expr `parser:"'filter' @@"`
}

// Order is one ORDER BY expression.
type Order struct {
	Expr *Expr  `parser:"@@"`
	Dir  string `parser:"@( 'asc' | 'desc' )?"`
}

// Expr is an AQL expression with an optional binary operation.
type Expr struct {
	Left  *Primary `parser:"@@"`
	Op    string   `parser:"( @( '!=' | '<=' | '>=' | '=' | '<' | '>' | 'and' | 'or' | '??' | 'in' | 'like' | 'ilike' )"`
	Right *Primary `parser:"@@ )?"`
}

// Primary is a single expression operand.
type Primary struct {
	// Subquery: (select TypeName { shape } filter ...)
	// Must come before SubExpr so that '(' 'select' is matched here, not as an expr.
	SubQuery *SelectBody `parser:"  '(' 'select' @@ ')'"`
	// Sub-insert: (insert TypeName { field := expr, ... })
	// Must come before SubExpr so that '(' 'insert' is matched here.
	SubInsert *InsertBody `parser:"| '(' @@ ')'"`
	// Sub-expression or parenthesized expression: (expr)
	SubExpr  *Expr       `parser:"| '(' @@ ')'"`
	// Function call: count(...)
	FuncCall *FuncCall `parser:"| @@"`
	// Path expression: .email or .author.name
	Path     *PathExpr `parser:"| @@"`
	// Parameter: $email or $email? (optional)
	Param    *Param    `parser:"| @@"`
	// Null literal
	Null     bool      `parser:"| @'null'"`
	// Bool literals
	True     bool      `parser:"| @'true'"`
	False    bool      `parser:"| @'false'"`
	// String literal
	Str      *string   `parser:"| @String"`
	// Integer literal
	Int      *string   `parser:"| @Int"`
	// Float literal
	Float    *string   `parser:"| @Float"`
	// Qualified identifier: TypeName.field (e.g. User.id in a subquery filter).
	// Must come before Ident so the parser greedily consumes TypeName.field as one node.
	QualifiedIdent *QualifiedIdent `parser:"| @@"`
	// Bare identifier (enum value, type name, etc.)
	Ident    *string   `parser:"| @Ident"`
}

// Param is a query parameter: $email (required) or $email? (optional).
// An optional param compiles to a filter condition that is skipped when the
// value is null, and becomes a nullable type in generated code.
//
// An optional inline type annotation ($email<str>, $limit<int32>?) names the
// param's type explicitly. The type may be any declared ASL value type — a
// builtin scalar, a scalar alias, or an enum — but not an object type.
type Param struct {
	Name     string `parser:"'$' @Ident"`
	Type     string `parser:"( '<' @Ident '>' )?"`
	Optional bool   `parser:"@'?'?"`
}

// FuncCall: funcName(expr, ...)
type FuncCall struct {
	Name string  `parser:"@Ident '('"`
	Args []*Expr `parser:"( @@ ( ',' @@ )* )? ')'"`
}

// PathExpr is a dotted path: .email / .author.name
type PathExpr struct {
	Steps []string `parser:"( '.' @Ident )+"`
}
