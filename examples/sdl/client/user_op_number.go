package ax

import "fmt"

type UserOpNumber[T any] struct {
	field string
}

func NewUserOpNumber[T any](field string) *UserOpNumber[T] {
	return &UserOpNumber[T]{field: field}
}

func (o *UserOpNumber[T]) Eq(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpNumber[T]) NotEq(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpNumber[T]) Gt(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Gt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpNumber[T]) Gte(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Gte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpNumber[T]) Lt(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Lt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpNumber[T]) Lte(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Lte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}
