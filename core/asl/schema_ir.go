package asl

// SchemaIR is the resolved, fully-typed representation of a .asl schema.
// It is produced by the Resolver from a parsed SourceFile.
type SchemaIR struct {
	ScalarTypes map[string]*ResolvedScalar
	EnumTypes   map[string]*ResolvedEnum
	ObjectTypes map[string]*ResolvedType
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
}

// ResolvedProp is a resolved scalar property.
type ResolvedProp struct {
	Name        string
	Column      string // snake_case column name
	SQLType     string // "TEXT", "INTEGER", "BOOLEAN", "UUID", "TIMESTAMPTZ"
	IsRequired  bool
	IsMulti     bool   // true → array or junction table
	Default     string // SQL default expression
	Constraints []ResolvedConstraint
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
