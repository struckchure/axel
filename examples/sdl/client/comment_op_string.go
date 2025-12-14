package ax

import (
	"fmt"
	lo "github.com/samber/lo"
)

type CommentOpString struct {
	field string
}

func NewCommentOpString(field string) *CommentOpString {
	return &CommentOpString{field: field}
}

func (o *CommentOpString) Eq(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpString) NotEq(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpString) Contains(value string, sensitive bool) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v%%", value),
	}
}

func (o *CommentOpString) StartsWith(value string, sensitive bool) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%v%%", value),
	}
}

func (o *CommentOpString) EndsWith(value string, sensitive bool) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v", value),
	}
}
