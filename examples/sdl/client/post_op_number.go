package ax

import "fmt"

type PostOpNumber[T any] struct {
	field string
}

func NewPostOpNumber[T any](field string) *PostOpNumber[T] {
	return &PostOpNumber[T]{field: field}
}

func (o *PostOpNumber[T]) Eq(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpNumber[T]) NotEq(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpNumber[T]) Gt(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Gt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpNumber[T]) Gte(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Gte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpNumber[T]) Lt(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Lt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpNumber[T]) Lte(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Lte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}
