package clients

import (
	"fmt"

	"github.com/samber/lo"
)

type UserOpString[T any] struct {
	UserOpBool[T]

	Field string
}

func NewUserOpString(field string) *UserOpString[string] {
	op := &UserOpString[string]{Field: field}

	op.UserOpBool = UserOpBool[string]{field: field}

	return op
}

func (o *UserOpString[T]) Contains(value T, sensitive bool) *UserOp {
	return &UserOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		value:    fmt.Sprintf("%%%v%%", value),
		required: true,
	}
}

func (o *UserOpString[T]) StartsWith(value T, sensitive bool) *UserOp {
	return &UserOp{column: o.Field, operator: lo.Ternary(sensitive, Like, Ilike), value: fmt.Sprintf("%v%%", value), required: true}
}

func (o *UserOpString[T]) EndsWith(value T, sensitive bool) *UserOp {
	return &UserOp{column: o.Field, operator: lo.Ternary(sensitive, Like, Ilike), value: fmt.Sprintf("%%%v", value), required: true}
}
