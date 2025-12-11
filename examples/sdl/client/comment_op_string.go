package ax

import (
	"fmt"
	lo "github.com/samber/lo"
)

type CommentOpString struct {
	Field string
}

func NewCommentOpString(field string) *CommentOpString {
	return &CommentOpString{Field: field}
}

func (o *CommentOpString) Eq(value string) *CommentOp {
	return &CommentOp{
		column:   o.Field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpString) NotEq(value string) *CommentOp {
	return &CommentOp{
		column:   o.Field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpString) Contains(value string, sensitive bool) *CommentOp {
	return &CommentOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v%%", value),
	}
}

func (o *CommentOpString) StartsWith(value string, sensitive bool) *CommentOp {
	return &CommentOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%v%%", value),
	}
}

func (o *CommentOpString) EndsWith(value string, sensitive bool) *CommentOp {
	return &CommentOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v", value),
	}
}
