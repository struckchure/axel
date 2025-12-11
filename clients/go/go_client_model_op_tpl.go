package clients

type UserOp struct {
	column   string
	operator Operator
	value    string
	required bool
}
