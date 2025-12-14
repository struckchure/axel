package ax

import "fmt"

type CommentOpNumber[T any] struct {
	field string
}

func NewCommentOpNumber[T any](field string) *CommentOpNumber[T] {
	return &CommentOpNumber[T]{field: field}
}

func (o *CommentOpNumber[T]) Eq(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpNumber[T]) NotEq(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpNumber[T]) Gt(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Gt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpNumber[T]) Gte(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Gte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpNumber[T]) Lt(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Lt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *CommentOpNumber[T]) Lte(value string) *CommentOp {
	return &CommentOp{
		column:   o.field,
		operator: Lte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}
