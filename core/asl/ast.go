package asl

import "github.com/alecthomas/participle/v2/lexer"

// SourceFile is the root AST node for a .asl schema file.
type SourceFile struct {
	Definitions []*Definition `parser:"@@*"`
}

// Definition is any top-level declaration.
type Definition struct {
	ScalarType *ScalarTypeDef `parser:"  @@"`
	EnumType   *EnumTypeDef   `parser:"| @@"`
	Function   *FunctionDecl  `parser:"| @@"`
	TypeDef    *TypeDef       `parser:"| @@"`
}

// ScalarTypeDef defines a named scalar alias.
//
//	scalar type EmailStr extending str;
type ScalarTypeDef struct {
	Pos     lexer.Position
	Name    string `parser:"'scalar' 'type' @Ident"`
	Extends string `parser:"'extending' @Ident ';'"`
	EndPos  lexer.Position
}

// EnumTypeDef defines an enum type.
//
//	enum Role { Admin, Member, Guest }
type EnumTypeDef struct {
	Pos    lexer.Position
	Name   string   `parser:"'enum' @Ident '{'"`
	Values []string `parser:"@Ident ( ',' @Ident )* ','? '}'"`
	EndPos lexer.Position
}

// TypeDef defines a concrete or abstract type (or model — backward compat).
//
//	abstract type User extending Timestamped { ... }
//	model User extends Base { ... }
type TypeDef struct {
	Pos       lexer.Position
	Abstract  bool      `parser:"@'abstract'?"`
	Name      string    `parser:"( 'type' | 'model' ) @Ident"`
	Extending []string  `parser:"( ( 'extends' | 'extending' ) @Ident ( ',' @Ident )* )?"`
	Members   []*Member `parser:"'{' @@* '}'"`
	EndPos    lexer.Position
}

// Member is any declaration inside a type body.
type Member struct {
	Computed   *ComputedDecl   `parser:"  @@"`
	Index      *IndexDecl      `parser:"| @@"`
	Constraint *ConstraintDecl `parser:"| @@"`
	Trigger    *TriggerDecl    `parser:"| @@"`
	Field      *FieldDecl      `parser:"| @@"`
}

// FieldDecl covers both property and link declarations.
//
// Link:      required link author: User;
// Multi:     multi link likes: User;
// Property:  required property email: str { constraint exclusive; };
// Bare prop: required age: int32;
type FieldDecl struct {
	Pos         lexer.Position
	Required    bool      `parser:"@'required'?"`
	Multi       bool      `parser:"@'multi'?"`
	Single      bool      `parser:"@'single'?"`
	PropKeyword bool      `parser:"@'property'?"`
	LinkKeyword bool      `parser:"@'link'?"`
	Name        string    `parser:"@Ident"`
	TypeSpec    *TypeSpec `parser:"@@?"`
	// ";" is always consumed here — attached after optional body
	Body   *FieldBody `parser:"( '{' @@ '}' )? ';'"`
	EndPos lexer.Position
}

// TypeSpec holds the type annotation for a field.
//
//	email: str        property
//	author: User      link (when target is an object type)
type TypeSpec struct {
	PropType *string `parser:"':' @Ident"`
}

// FieldBody holds the constraint, default, and on-clause items for a field.
type FieldBody struct {
	Items []*FieldBodyItem `parser:"@@*"`
}

// FieldBodyItem is one item inside a field's body block.
type FieldBodyItem struct {
	Constraint *FieldConstraintDecl `parser:"  @@"`
	Rewrite    *RewriteDecl         `parser:"| @@"`
	Default    *DefaultDecl         `parser:"| @@"`
	OnClause   *OnClause            `parser:"| @@"`
}

// RewriteDecl is a field-level rewrite — sugar that folds into a BEFORE trigger
// that assigns the field on the named events.
//
//	rewrite update := datetime_current();      # builtin function → now()
//	rewrite insert, update := __new__.slug      # a column of the row being written
//	rewrite update := __old__.status            # the pre-update row (UPDATE only)
//	rewrite update := 'edited'                  # a literal
//
// Row references use the same magic identifiers as triggers: __new__ / __old__ /
// __subject__ (an alias for __new__). Events are insert / update (create is
// accepted as an alias for insert).
type RewriteDecl struct {
	Pos    lexer.Position
	Events []string `parser:"'rewrite' @Ident ( ',' @Ident )* ':='"`
	Func   *string  `parser:"( @Ident '(' ')'"`
	Row    *string  `parser:"| @Ident '.'"`
	Field  *string  `parser:"@Ident"`
	Lit    *string  `parser:"| @( String | Int ) ) ';'?"`
	EndPos lexer.Position
}

// FieldConstraintDecl: constraint exclusive; / constraint min_length(10);
// Trailing semicolon is optional (some single-item bodies omit it).
type FieldConstraintDecl struct {
	Pos  lexer.Position
	Name string   `parser:"'constraint' @Ident"`
	Args []string `parser:"( '(' @( Ident | Int | String ) ( ',' @( Ident | Int | String ) )* ')' )? ';'?"`
}

// DefaultDecl supports both old and new default syntax.
// Trailing semicolon is optional (single-item bodies may omit it).
//
//	New:  default := gen_uuid();  / default := true  / default := 'n/a';
//	Old:  default @func(gen_random_uuid);  / default 'n/a'  / default true
type DefaultDecl struct {
	// new: default := funcName()
	NewFunc *string `parser:"  'default' ':=' @Ident '(' ')' ';'?"`
	// new: default := Enum.Member (qualified enum reference)
	QualEnum []string `parser:"| 'default' ':=' @Ident '.' @Ident ';'?"`
	// new: default := literal
	NewLit *string `parser:"| 'default' ':=' @( String | Int | Ident ) ';'?"`
	// old: default @func(name)
	OldFunc *string `parser:"| 'default' '@' 'func' '(' @Ident ')' ';'?"`
	// old: default literal
	OldLit *string `parser:"| 'default' @( String | Int | Ident ) ';'?"`
}

// OnClause specifies the join field for old-style links.
//
//	on id; / on email
type OnClause struct {
	Field string `parser:"'on' @Ident ';'?"`
}

// ComputedDecl defines a computed (derived) property.
//
//	computed display_name := .name ?? .email;
type ComputedDecl struct {
	Pos   lexer.Position
	Name  string   `parser:"'computed' @Ident ':='"`
	Parts []string `parser:"@( Ident | '.' | '??' | String | Int )+ ';'"`
}

// ConstraintDecl declares an constraint on one or more properties.
//
//	constraint <expression> on (.email);
//	constraint <expression> on (.active, .age);
type ConstraintDecl struct {
	Pos        lexer.Position
	Expression string   `parser:"'constraint' @Ident"`
	Args       []string `parser:"( '(' @( Ident | Int | String ) ( ',' @( Ident | Int | String ) )* ')' )?"`
	Fields     []string `parser:"'on' '(' '.' @Ident ( ',' '.' @Ident )* ')' ';'"`
}

// IndexDecl declares an index on one or more properties.
//
//	index on (.email);
//	index on (.active, .age);
type IndexDecl struct {
	Pos    lexer.Position
	Fields []string `parser:"'index' 'on' '(' '.' @Ident ( ',' '.' @Ident )* ')' ';'"`
}

// TriggerDecl declares a row/statement trigger on the enclosing type. It has two
// mutually-exclusive bodies: an inline AQL `do ( … )`, or `execute <fn>()`.
//
//	trigger audit after insert, update, delete do ( insert AuditLog { … } );
//	trigger touch before update execute my_fn();
type TriggerDecl struct {
	Pos     lexer.Position
	Name    string    `parser:"'trigger' @Ident"`
	Timing  string    `parser:"@( 'before' | 'after' )"`
	Events  []string  `parser:"@Ident ( ',' @Ident )*"`
	ForEach string    `parser:"( 'for' 'each' @( 'row' | 'statement' ) )?"`
	When    *string   `parser:"( 'when' '(' @DollarString ')' )?"`
	Do      *AQLBlock `parser:"( 'do' @@"`
	ExecFn  *string   `parser:"| 'execute' @Ident '(' ')' )"`
	End     string    `parser:"';'"`
	EndPos  lexer.Position
}

// FunctionDecl is a top-level Postgres function. Its body is either raw SQL
// (dollar-quoted) or an inline AQL statement.
//
//	function touch() -> trigger { body := ( update … ); };
//	function my_fn() -> trigger { language := plpgsql; body := $$ … $$; };
type FunctionDecl struct {
	Pos     lexer.Position
	Name    string       `parser:"'function' @Ident '('"`
	Params  []*FuncParam `parser:"( @@ ( ',' @@ )* )? ')'"`
	Returns string       `parser:"'->' @Ident '{'"`
	Items   []*FuncItem  `parser:"@@* '}' ';'?"`
	EndPos  lexer.Position
}

// FuncParam is one `name: type` parameter of a function.
type FuncParam struct {
	Name string `parser:"@Ident ':'"`
	Type string `parser:"@Ident"`
}

// FuncItem is one item in a function body block: `language := ident` or
// `body := ( aql )` / `body := $$ sql $$`.
type FuncItem struct {
	Language *string   `parser:"  'language' ':=' @Ident ';'?"`
	BodySQL  *string   `parser:"| 'body' ':=' @DollarString ';'?"`
	BodyAQL  *AQLBlock `parser:"| 'body' ':=' @@ ';'?"`
}
