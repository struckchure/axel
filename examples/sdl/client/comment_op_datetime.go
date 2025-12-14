package ax

import "fmt"

type CommentOpDatetime struct {
	field string
}

func NewCommentOpDatetime(field string) *CommentOpDatetime {
	return &CommentOpDatetime{field: field}
}

func (o *CommentOpDatetime) Eq(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpDatetime) NotEq(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpDatetime) Gt(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Gt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpDatetime) Gte(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Gte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpDatetime) Lt(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Lt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpDatetime) Lte(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Lte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}
