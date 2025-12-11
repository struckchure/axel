package ax

type UserOp struct {
	column   string
	operator Operator
	value    string
	required bool
}
