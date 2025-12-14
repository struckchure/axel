package ax

import "fmt"

type CommentOpBool struct {
	field string
}

func NewCommentOpBool(field string) *CommentOpBool {
	return &CommentOpBool{field: field}
}

func (o *CommentOpBool) Eq(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpBool) NotEq(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}
