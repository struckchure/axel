package clients

import "fmt"

type UserOpNumber[T any] struct {
	UserOpBool[T]

	Field string
}

func NewUserOpNumber[T any](field string) *UserOpNumber[T] {
	op := &UserOpNumber[T]{Field: field}

	op.UserOpBool.field = op.Field

	return op
}
func (o *UserOpNumber[T]) Gt(value T) *UserOp {
	return &UserOp{column: o.Field, operator: Gt, value: fmt.Sprintf("%v", value), required: true}
}
func (o *UserOpNumber[T]) Gte(value T) *UserOp {
	return &UserOp{column: o.Field, operator: Gte, value: fmt.Sprintf("%v", value), required: true}
}
func (o *UserOpNumber[T]) Lt(value T) *UserOp {
	return &UserOp{column: o.Field, operator: Lt, value: fmt.Sprintf("%v", value), required: true}
}
func (o *UserOpNumber[T]) Lte(value T) *UserOp {
	return &UserOp{column: o.Field, operator: Lte, value: fmt.Sprintf("%v", value), required: true}
}
