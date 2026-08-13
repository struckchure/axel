package asl

// SchemaIR is the resolved, fully-typed representation of a .asl schema.
// It is produced by the Resolver from a parsed SourceFile.
type SchemaIR struct {
	ScalarTypes map[string]*ResolvedScalar
	EnumTypes   map[string]*ResolvedEnum
	ObjectTypes map[string]*ResolvedType
	Functions   map[string]*ResolvedFunction
	Extensions  []string          // declared Postgres extensions, in declaration order (deduped)
	Globals     []*ResolvedGlobal // declared global variables, in declaration order
}

// ResolvedGlobal is a resolved global variable. It carries no DDL — a global is a
// Postgres session setting (custom GUC `app.<Name>`), read via current_setting.
// AQLType keeps the declared ASL scalar name (for codegen typing); SQLType is the
// Postgres type its value casts to; Required controls current_setting's missing_ok
// (required → error when unset, optional → NULL when unset).
type ResolvedGlobal struct {
	Name     string
	AQLType  string // declared ASL scalar name, e.g. "uuid"
	SQLType  string // resolved SQL type, e.g. "UUID"
	Required bool
}

// ResolvedScalar is a scalar type alias (e.g. EmailStr extending str).
type ResolvedScalar struct {
	Name    string
	Base    string // the builtin type it extends: "str", "int32", etc.
	SQLType string // the SQL type to use: "TEXT", "INTEGER", etc.
}

// ResolvedEnum is an enum type.
type ResolvedEnum struct {
	Name   string
	Values []string
}

// ResolvedType is a resolved object type (abstract or concrete).
type ResolvedType struct {
	Name        string
	IsAbstract  bool
	Table       string // snake_case table name; empty for abstract types
	Properties  map[string]*ResolvedProp
	Links       map[string]*ResolvedLink
	Computed    map[string]*ResolvedComputed
	Indexes     []*ResolvedIndex
	Constraints []*ResolvedTypeConstraint
	Triggers    []*ResolvedTrigger
	Policies    []*ResolvedPolicy
}

// ResolvedPolicy is a resolved row-level-security policy on a type. UsingAQL /
// CheckAQL are the raw AQL predicate source (native AQL, not yet lowered to SQL);
// "" means the clause was omitted. Lowering to SQL is deferred to the migration
// bridge (SchemaIRToPolicies), which parses the AQL and compiles it — this keeps
// the asl package free of any AQL/compiler dependency.
type ResolvedPolicy struct {
	Name     string
	Commands []string // each one of "select" | "insert" | "update" | "delete" | "all"
	Roles    []string // target roles; empty → PUBLIC
	UsingAQL string   // raw AQL USING predicate; "" if none
	CheckAQL string   // raw AQL WITH CHECK predicate; "" if none
}

// ResolvedTrigger is a resolved row/statement trigger on a type. Exactly one of
// DoAQL / Function is set (inline AQL body vs. reference to a declared function).
type ResolvedTrigger struct {
	Name     string
	Timing   string   // "before" | "after"
	Events   []string // "insert" | "update" | "delete"
	ForEach  string   // "row" | "statement" (defaults to "row")
	When     string   // raw SQL condition (interior of $$…$$); "" if none
	DoAQL    string   // raw AQL statement for an inline body; "" if execute-form
	Function string   // name of a declared function to execute; "" if inline
}

// ResolvedFunction is a resolved top-level Postgres function.
type ResolvedFunction struct {
	Name      string
	Params    []ResolvedFuncParam
	Returns   string // SQL type, or "trigger"
	Language  string // "plpgsql" (default) | "sql"
	ReturnSQL string // raw SQL expression from `return <expr>;`; wrapped by the lowerer

	// InlineAQL holds the raw source of each aql`…` literal in ReturnSQL, in
	// order; ReturnSQL carries one NUL byte per entry where the compiled SQL
	// string literal goes. The lowerer in package axel does the substitution.
	InlineAQL []string

	// Attributes, from leading @-directives.
	Volatility string // "immutable" | "stable" | "volatile" | ""
	Strict     bool
	Leakproof  bool
	Parallel   string // "safe" | "unsafe" | "restricted" | ""
	Security   string // "definer" | "invoker" | ""
	Cost       string // numeric cost, e.g. "100"; "" if unset

	// RunOnceFor names the type an @for function is a one-time setup for. When
	// set, the migration that first creates the function also invokes it once
	// (e.g. to register a pg_cron job). "" for ordinary functions.
	RunOnceFor string
}

// ResolvedFuncParam is one resolved function parameter.
type ResolvedFuncParam struct {
	Name    string
	SQLType string
}

// ResolvedProp is a resolved scalar property.
type ResolvedProp struct {
	Name        string
	Column      string // snake_case column name
	SQLType     string // "TEXT", "INTEGER", "BOOLEAN", "UUID", "TIMESTAMPTZ"
	EnumType    string // enum type name when the property is enum-backed; "" otherwise
	IsRequired  bool
	IsMulti     bool   // true → array or junction table
	Default     string // SQL default expression
	Constraints []ResolvedConstraint
	Rewrites    []ResolvedRewrite // field-level rewrites → folded into a BEFORE trigger
}

// ResolvedRewrite is a resolved field rewrite: on the given events, assign the
// resolved SQL value expression to the field's column.
type ResolvedRewrite struct {
	Events   []string // "insert" | "update"
	ValueSQL string   // the SQL assigned to NEW.<column> (e.g. "now()", `NEW."slug"`)
	Origin   string   // name of the type that declared the rewrite (drives the shared function name)
}

// ResolvedLink is a resolved object link (FK or junction table).
type ResolvedLink struct {
	Name          string
	TargetType    string // name of the target ResolvedType
	JoinColumn    string // FK column in this table: "author_id" (single links)
	JoinField     string // the target field referenced: "id", "email"
	JunctionTable string // junction table name for multi links: "post_tags"
	IsRequired    bool
	IsMulti       bool
	Constraints   []ResolvedConstraint // body constraints on the link column (e.g. exclusive)
}

// ResolvedComputed is a computed/derived property (not stored in DB).
type ResolvedComputed struct {
	Name string
	Expr string // raw expression parts joined
}

// ResolvedIndex is a resolved index on one or more columns.
type ResolvedIndex struct {
	Columns []string // column names (snake_case)
}

// ResolvedConstraint is a resolved field constraint.
type ResolvedConstraint struct {
	Name string
	Args []string
}

// ResolvedTypeConstraint is a resolved type-level constraint spanning one or
// more columns (e.g. composite UNIQUE / PRIMARY KEY, or a length CHECK).
type ResolvedTypeConstraint struct {
	Expression string   // "exclusive", "pk", "min_length", "max_length"
	Args       []string // e.g. ["6"] for length constraints
	Columns    []string // snake_case column names the constraint applies to
	FilterAQL  string   // raw AQL partial predicate; "" if not partial. Lowered
	// to a `CREATE UNIQUE INDEX … WHERE` in the migration bridge.
}
