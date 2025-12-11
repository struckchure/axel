package ax

type Operator string

const (
	Eq    Operator = "="
	NotEq Operator = "!="
	Gt    Operator = ">"
	Gte   Operator = ">="
	Lt    Operator = "<"
	Lte   Operator = "<="
	Like  Operator = "LIKE"
	Ilike Operator = "ILIKE"
	In    Operator = "IN"
)
