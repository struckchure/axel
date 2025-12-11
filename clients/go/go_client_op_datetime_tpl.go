package clients

import (
	"fmt"
	"time"
)

type UserOpDateTime[T any] struct {
	field string
}

func NewUserOpDateTime(field string) *UserOpDateTime[time.Time] {
	return &UserOpDateTime[time.Time]{field: field}
}

func (o *UserOpDateTime[T]) Eq(value T) *UserOp {
	return &UserOp{column: o.field, operator: Eq, value: fmt.Sprintf("%v", value), required: true}
}

func (o *UserOpDateTime[T]) NotEq(value T) *UserOp {
	return &UserOp{column: o.field, operator: NotEq, value: fmt.Sprintf("%v", value), required: true}
}
