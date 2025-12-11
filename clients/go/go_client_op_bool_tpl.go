package clients

import (
	"fmt"
)

type UserOpBool[T any] struct {
	field string
}

func NewUserOpBool(field string) *UserOpBool[bool] {
	return &UserOpBool[bool]{field: field}
}

func (o *UserOpBool[T]) Eq(value T) *UserOp {
	return &UserOp{column: o.field, operator: Eq, value: fmt.Sprintf("%v", value), required: true}
}

func (o *UserOpBool[T]) NotEq(value T) *UserOp {
	return &UserOp{column: o.field, operator: NotEq, value: fmt.Sprintf("%v", value), required: true}
}
