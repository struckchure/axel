package ax

type PostOp struct {
	column   string
	operator Operator
	value    string
	required bool
}
