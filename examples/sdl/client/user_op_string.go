package ax

import (
	"fmt"
	lo "github.com/samber/lo"
)

type UserOpString struct {
	Field string
}

func NewUserOpString(field string) *UserOpString {
	return &UserOpString{Field: field}
}

func (o *UserOpString) Eq(value string) *UserOp {
	return &UserOp{
		column:   o.Field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpString) NotEq(value string) *UserOp {
	return &UserOp{
		column:   o.Field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpString) Contains(value string, sensitive bool) *UserOp {
	return &UserOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v%%", value),
	}
}

func (o *UserOpString) StartsWith(value string, sensitive bool) *UserOp {
	return &UserOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%v%%", value),
	}
}

func (o *UserOpString) EndsWith(value string, sensitive bool) *UserOp {
	return &UserOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v", value),
	}
}
