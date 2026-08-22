package aql

import (
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// Statement is the top-level AQL query node.
// Optional directives may precede the statement; exactly one of the statement
// fields will be non-nil per parsed statement.
type Statement struct {
	Pos        lexer.Position
	Directives []*Directive `parser:"@@*"`
	Vars       []*VarBlock  `parser:"@@*"`
	With       *WithBlock   `parser:"@@?"`
	Select     *SelectStmt  `parser:"( @@"`
	Insert     *InsertStmt  `parser:"| @@"`
	Update     *UpdateStmt  `parser:"| @@"`
	Delete     *DeleteStmt  `parser:"| @@ )"`
	EndPos     lexer.Position
}

// VarBlock is a leading `var ( ... )` or `var $param...;` block that declares
// named query parameters and their explicit types / optionality.
//
//	var (
//	  $currency<str>?;
//	  $api_key_id<uuid>?;
//	)
//	var $limit<int32>?;
type VarBlock struct {
	Pos    lexer.Position
	Params []*Param `parser:"'var' ( '(' ( @@ ';'? )* ')' ';'? | @@ ';' )"`
	EndPos lexer.Position
}

// WithBlock is a leading `with ( name := (select ...); ... )` clause that binds
// named subqueries for the statement that follows. Each binding lowers to a
// Postgres CTE, so a sub-select reused at several points in the filter is
// evaluated once instead of being inlined per use site.
//
//	with (
//	  business := (select Business filter .id = $business_id);
//	  api_keys := (multi select ApiKey filter .business = $business_id);
//	)
//	multi select Transaction filter .sender_id in api_keys.id;
//
// A binding is referenced by name: bare (`business`, meaning its id) or
// qualified (`business.id`, `api_keys.id`). Both forms already parse as
// Primary.Ident / Primary.QualifiedIdent — the compiler resolves them against
// the block's bindings, which shadow type names of the same spelling.
type WithBlock struct {
	Pos      lexer.Position
	Bindings []*WithBinding `parser:"'with' '(' ( @@ ( ';' | ',' )? )* ')' ';'? "`
	EndPos   lexer.Position
}

// WithBinding is one `name := expr` entry in a WithBlock. Value is a full Expr
// rather than a dedicated subquery rule so `(select ...)` and `(multi select
// ...)` reuse Primary's existing subquery parsing; the compiler validates that
// the value really is a solo subquery and reports a precise error when it isn't.
type WithBinding struct {
	Pos    lexer.Position
	Name   string `parser:"@Ident ':='"`
	Value  *Expr  `parser:"@@"`
	EndPos lexer.Position
}

// Directive is a leading `@name value` declaration that carries codegen metadata
// (e.g. @response User, @request CreateUserInput, @name CreateUser). Unknown
// directives are parsed and preserved but ignored by the compiler.
type Directive struct {
	Pos    lexer.Position
	Name   string `parser:"'@' @Ident"`
	Value  string `parser:"@( Ident | String | Int )"`
	EndPos lexer.Position
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
	Pos   lexer.Position
	Multi bool        `parser:"@'multi'? 'select'"`
	Body  *SelectBody `parser:"@@"`
	End   string      `parser:"';'?"`
}

// SelectBody holds the select content — either an aggregate or a typed shape query.
// Wrapping in a sub-struct ensures the ";" terminator in SelectStmt is always consumed
// regardless of which alternative wins inside SelectBody.
type SelectBody struct {
	// Aggregate: count(TypeName filter expr)
	AggFunc *AggExpr `parser:"  @@"`
	// Object: TypeName { shape } filter ... group by ... having ... order by ... limit ... offset ...
	TypeName string     `parser:"| @Ident"`
	Shape    *Shape     `parser:"@@?"`
	Filter   *Filter    `parser:"@@?"`
	GroupBy  []*GroupBy `parser:"( 'group' 'by' @@ ( ',' @@ )* )?"`
	Having   *Having    `parser:"@@?"`
	OrderBy  []*Order   `parser:"( 'order' 'by' @@ ( ',' @@ )* )?"`
	Limit    *Expr      `parser:"( 'limit' @@ )?"`
	Offset   *Expr      `parser:"( 'offset' @@ )?"`
}

// GroupBy is one GROUP BY expression.
type GroupBy struct {
	Pos    lexer.Position
	Expr   *Expr `parser:"@@"`
	EndPos lexer.Position
}

// Having is a HAVING condition on grouped rows.
type Having struct {
	Pos    lexer.Position
	Expr   *Expr `parser:"'having' @@"`
	EndPos lexer.Position
}

// AggExpr: count(TypeName filter expr)
type AggExpr struct {
	Func     string  `parser:"@Ident '('"`
	TypeName string  `parser:"@Ident"`
	Filter   *Filter `parser:"@@?"`
	End      string  `parser:"')'"`
}

// InsertStmt: insert TypeName { field := expr, ... } [unless conflict [on ...] [else (...)]];
type InsertStmt struct {
	Pos         lexer.Position
	TypeName    string        `parser:"'insert' @Ident"`
	Assignments []*Assignment `parser:"'{' @@ ( ',' @@ )* ','? '}'"`
	Conflict    *OnConflict   `parser:"( 'unless' 'conflict' @@ )?"`
	End         string        `parser:"';'?"`
}

// OnConflict is the upsert clause on an insert. It lowers to Postgres
// `ON CONFLICT [(cols)] DO NOTHING` (no Else) or `ON CONFLICT (cols) DO UPDATE
// SET ...` (with Else).
//
//	unless conflict
//	unless conflict on .email
//	unless conflict on (.a, .b)
//	unless conflict on .email else (update User set { name := $name })
type OnConflict struct {
	Target *ConflictTarget `parser:"( 'on' @@ )?"`
	Else   *ConflictUpdate `parser:"( 'else' '(' @@ ')' )?"`
}

// ConflictTarget names the exclusive field(s) that define the conflict:
// `.field` or `(.a, .b)` for a composite constraint.
type ConflictTarget struct {
	Fields []string `parser:"( '.' @Ident | '(' '.' @Ident ( ',' '.' @Ident )* ')' )"`
}

// ConflictUpdate is the `else (update TypeName set { ... })` arm, lowered to the
// `DO UPDATE SET ...` clause. The TypeName must match the insert's type and no
// filter is allowed (Postgres targets the conflicting row automatically).
type ConflictUpdate struct {
	TypeName    string        `parser:"'update' @Ident"`
	Assignments []*Assignment `parser:"'set' '{' @@ ( ',' @@ )* ','? '}'"`
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
	Pos         lexer.Position
	TypeName    string        `parser:"'update' @Ident"`
	Filter      *Filter       `parser:"@@?"`
	Assignments []*Assignment `parser:"'set' '{' @@ ( ',' @@ )* ','? '}'"`
	End         string        `parser:"';'?"`
}

// DeleteStmt: delete TypeName filter expr;
type DeleteStmt struct {
	Pos      lexer.Position
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
//   - → splat: all scalar props + single-link FK columns
//     id               → leaf field
//     posts: { title } → nested link with sub-shape
//     posts := (...)   → inline computed field
//     total := sum(.amount) filter .status = X → aggregate field (see AggFilter)
type ShapeField struct {
	Pos      lexer.Position
	Star     bool   `parser:"(   @'*'"`
	Name     string `parser:"  | @Ident )"`
	SubShape *Shape `parser:"( ':' @@ )?"`
	Computed *Expr  `parser:"( ':=' @@ )?"`
	// AggFilter is a per-field `filter <cond>` tail, valid only on an aggregate
	// field in an aggregation select. It lowers to SQL `FILTER (WHERE <cond>)` on
	// the aggregate. Because `Filter` begins with the `filter` keyword, the tail
	// parses unambiguously after `Computed` — an inner subquery's own filter stays
	// inside its parens, so `name := (select … filter …)` is unaffected.
	AggFilter *Filter `parser:"@@?"`
}

// AggFuncs is the set of aggregate functions valid as an aggregation-select
// shape value. Names are matched case-insensitively.
var AggFuncs = map[string]bool{
	"count": true,
	"sum":   true,
	"avg":   true,
	"min":   true,
	"max":   true,
}

// AggCall returns the aggregate FuncCall and its trailing `<Type>` cast when this
// shape field is an aggregate value — a top-level call to an AggFuncs function
// (e.g. `sum(.amount)` or `sum(.amount)<int64>`), optionally with an AggFilter.
// It returns (nil, "", false) for any non-aggregate field. This is the single
// predicate the compiler and codegen use to recognise an aggregate field.
func (f *ShapeField) AggCall() (fc *FuncCall, cast string, ok bool) {
	if f == nil || f.Computed == nil {
		return nil, "", false
	}
	p := f.Computed.SoloPrimary()
	if p == nil || p.FuncCall == nil {
		return nil, "", false
	}
	if !AggFuncs[strings.ToLower(p.FuncCall.Name)] {
		return nil, "", false
	}
	return p.FuncCall, p.Cast, true
}

// QualifiedIdent is a TypeName.field or __new__.field.subfield reference used in expressions.
type QualifiedIdent struct {
	Pos      lexer.Position
	TypeName string   `parser:"@Ident '.'"`
	Field    string   `parser:"@Ident"`
	Fields   []string `parser:"( '.' @Ident )*"`
}

// Assignment is a field value assignment used in INSERT and UPDATE.
//
//	email := $email
type Assignment struct {
	Pos   lexer.Position
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

// Expr is a boolean expression: one or more and-groups joined by `or`.
//
// `and` binds tighter than `or`, so `a or b and c` means `a or (b and c)`.
// Parenthesize to override — a group is a Primary (see Primary.SubExpr), so it
// nests to any depth: (a or b) and (c or d) and e
//
// Pos/EndPos are carried on every expression node so the formatter can attach
// `#` comments to the right operand when it breaks a long boolean chain across
// lines (see format.go's expr/andExpr/cmp).
type Expr struct {
	Pos    lexer.Position
	Left   *AndExpr   `parser:"@@"`
	Rest   []*AndExpr `parser:"( 'or' @@ )*"`
	EndPos lexer.Position
}

// AndExpr is one or more comparisons joined by `and`.
type AndExpr struct {
	Pos    lexer.Position
	Left   *Cmp   `parser:"@@"`
	Rest   []*Cmp `parser:"( 'and' @@ )*"`
	EndPos lexer.Position
}

// Cmp is a single comparison, a postfix null-test (`.x is null` /
// `.x is not null`), or a bare operand when nothing follows Left. The null-test
// and the binary comparison are mutually-exclusive tails: `Is` marks the former
// (with `IsNot` for the negated form), `Op`/`Right` the latter.
type Cmp struct {
	Pos    lexer.Position
	Left   *Primary `parser:"@@"`
	Is     bool     `parser:"( @'is'"`
	IsNot  bool     `parser:"@'not'? 'null'"`
	Op     string   `parser:"| @( '!=' | '<=' | '>=' | '=' | '<' | '>' | '??' | 'in' | 'like' | 'ilike' )"`
	Right  *Primary `parser:"@@ )?"`
	EndPos lexer.Position
}

// SingleCmp returns the lone comparison when the expression does not chain
// and/or, else nil.
func (e *Expr) SingleCmp() *Cmp {
	if e == nil || len(e.Rest) != 0 || e.Left == nil || len(e.Left.Rest) != 0 {
		return nil
	}
	return e.Left.Left
}

// SoloPrimary returns the lone operand when the expression is a single operand
// with no operator (e.g. a bare `(select ...)` or `$param`), else nil.
func (e *Expr) SoloPrimary() *Primary {
	c := e.SingleCmp()
	if c == nil || c.Op != "" || c.Is {
		return nil
	}
	return c.Left
}

// Primary is a single expression operand, with an optional trailing `<Type>`
// cast that applies to whatever the operand is — a literal ('{}'<json>), a path
// (.a.b<uuid>), a parenthesized expression ((.a ?? .b)<str>), or a subquery
// projection ((select Org ...).slug<str>). The cast emits `(<sql>)::<sqltype>`
// and gives an otherwise un-inferable computed field a concrete type.
type Primary struct {
	Pos lexer.Position
	// Subquery: ( [multi] select TypeName { shape } filter ... )
	// Must come before SubExpr so that '(' 'select' is matched here, not as an expr.
	// A leading `multi` makes a computed shape field a JSON array; without it the
	// field is a single object (matching top-level select / multi select).
	// An optional trailing `.field` projects a single column from the subquery's
	// row instead of its id — e.g. (select Org filter .id = $id).slug
	SubQueryMulti bool        `parser:"( '(' @'multi'? 'select'"`
	SubQuery      *SelectBody `parser:"@@ ')'"`
	SubQueryField string      `parser:"( '.' @Ident )?"`
	// Sub-insert: (insert TypeName { field := expr, ... })
	// Must come before SubExpr so that '(' 'insert' is matched here.
	SubInsert *InsertBody `parser:"| '(' @@ ')'"`
	// Sub-expression or parenthesized expression: (expr)
	SubExpr *Expr `parser:"| '(' @@ ')'"`
	// Function call: count(...)
	FuncCall *FuncCall `parser:"| @@"`
	// Path expression: .email or .author.name
	Path *PathExpr `parser:"| @@"`
	// Parameter: $email or $email? (optional)
	Param *Param `parser:"| @@"`
	// Null literal
	Null bool `parser:"| @'null'"`
	// Bool literals
	True  bool `parser:"| @'true'"`
	False bool `parser:"| @'false'"`
	// String literal
	Str *string `parser:"| @String"`
	// Integer literal
	Int *string `parser:"| @Int"`
	// Float literal
	Float *string `parser:"| @Float"`
	// Global variable reference: `global current_user`. Lowers to
	// current_setting('app.<name>', …). Must come before Ident so `global` isn't
	// swallowed as a bare identifier.
	GlobalRef *string `parser:"| 'global' @Ident"`
	// Qualified identifier: TypeName.field (e.g. User.id in a subquery filter).
	// Must come before Ident so the parser greedily consumes TypeName.field as one node.
	QualifiedIdent *QualifiedIdent `parser:"| @@"`
	// Bare identifier (enum value, type name, etc.)
	Ident *string `parser:"| @Ident )"`
	// Optional `<Type>` cast applied to the operand above.
	Cast   string `parser:"( '<' @Ident '>' )?"`
	EndPos lexer.Position
}

// Param is a query parameter: $email (required) or $email? (optional).
// An optional param compiles to a filter condition that is skipped when the
// value is null, and becomes a nullable type in generated code.
//
// An optional inline type annotation ($email<str>, $limit<int32>?) names the
// param's type explicitly. The type may be any declared ASL value type — a
// builtin scalar, a scalar alias, or an enum — but not an object type.
type Param struct {
	Pos      lexer.Position
	Name     string `parser:"'$' @Ident"`
	Type     string `parser:"( '<' @Ident '>' )?"`
	Optional bool   `parser:"@'?'?"`
	EndPos   lexer.Position
}

// FuncCall: funcName(expr, ...)
type FuncCall struct {
	Name string  `parser:"@Ident '('"`
	Args []*Expr `parser:"( @@ ( ',' @@ )* )? ')'"`
}

// PathExpr is a dotted path: .email / .author.name
// A trailing `<Type>` cast, when present, is captured on the enclosing Primary.
type PathExpr struct {
	Pos   lexer.Position
	Steps []string `parser:"( '.' @Ident )+"`
}
