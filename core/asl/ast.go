package asl

// SourceFile is the root AST node for a .asl schema file.
type SourceFile struct {
	Definitions []*Definition `parser:"@@*"`
}

// Definition is any top-level declaration.
type Definition struct {
	ScalarType *ScalarTypeDef `parser:"  @@"`
	EnumType   *EnumTypeDef   `parser:"| @@"`
	TypeDef    *TypeDef       `parser:"| @@"`
}

// ScalarTypeDef defines a named scalar alias.
//
//	scalar type EmailStr extending str;
type ScalarTypeDef struct {
	Name    string `parser:"'scalar' 'type' @Ident"`
	Extends string `parser:"'extending' @Ident ';'"`
}

// EnumTypeDef defines an enum type.
//
//	enum Role { Admin, Member, Guest }
type EnumTypeDef struct {
	Name   string   `parser:"'enum' @Ident '{'"`
	Values []string `parser:"@Ident ( ',' @Ident )* ','? '}'"`
}

// TypeDef defines a concrete or abstract type (or model — backward compat).
//
//	abstract type User extending Timestamped { ... }
//	model User extends Base { ... }
type TypeDef struct {
	Abstract  bool      `parser:"@'abstract'?"`
	Name      string    `parser:"( 'type' | 'model' ) @Ident"`
	Extending []string  `parser:"( ( 'extends' | 'extending' ) @Ident ( ',' @Ident )* )?"`
	Members   []*Member `parser:"'{' @@* '}'"`
}

// Member is any declaration inside a type body.
type Member struct {
	Computed *ComputedDecl `parser:"  @@"`
	Index    *IndexDecl    `parser:"| @@"`
	Field    *FieldDecl    `parser:"| @@"`
}

// FieldDecl covers both property and link declarations.
//
// Link:      required link author: User;
// Multi:     multi link likes: User;
// Property:  required property email: str { constraint exclusive; };
// Bare prop: required age: int32;
type FieldDecl struct {
	Required    bool       `parser:"@'required'?"`
	Multi       bool       `parser:"@'multi'?"`
	Single      bool       `parser:"@'single'?"`
	PropKeyword bool       `parser:"@'property'?"`
	LinkKeyword bool       `parser:"@'link'?"`
	Name        string     `parser:"@Ident"`
	TypeSpec    *TypeSpec  `parser:"@@?"`
	// ";" is always consumed here — attached after optional body
	Body        *FieldBody `parser:"( '{' @@ '}' )? ';'"`
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
	Constraint *ConstraintDecl `parser:"  @@"`
	Default    *DefaultDecl    `parser:"| @@"`
	OnClause   *OnClause       `parser:"| @@"`
}

// ConstraintDecl: constraint exclusive; / constraint min_length(10);
// Trailing semicolon is optional (some single-item bodies omit it).
type ConstraintDecl struct {
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
	// new: default := literal
	NewLit  *string `parser:"| 'default' ':=' @( String | Int | Ident ) ';'?"`
	// old: default @func(name)
	OldFunc *string `parser:"| 'default' '@' 'func' '(' @Ident ')' ';'?"`
	// old: default literal
	OldLit  *string `parser:"| 'default' @( String | Int | Ident ) ';'?"`
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
	Name  string   `parser:"'computed' @Ident ':='"`
	Parts []string `parser:"@( Ident | '.' | '??' | String | Int )+ ';'"`
}

// IndexDecl declares an index on one or more properties.
//
//	index on (.email);
//	index on (.active, .age);
type IndexDecl struct {
	Fields []string `parser:"'index' 'on' '(' '.' @Ident ( ',' '.' @Ident )* ')' ';'"`
}
