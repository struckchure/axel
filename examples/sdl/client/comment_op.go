package ax

type CommentOp struct {
	column   string
	operator Operator
	value    string
	required bool
}
