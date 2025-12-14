package ax

import (
	"fmt"
	lo "github.com/samber/lo"
)

type UserOpString struct {
	field string
}

func NewUserOpString(field string) *UserOpString {
	return &UserOpString{field: field}
}

func (o *UserOpString) Eq(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpString) NotEq(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpString) Contains(value string, sensitive bool) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v%%", value),
	}
}

func (o *UserOpString) StartsWith(value string, sensitive bool) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%v%%", value),
	}
}

func (o *UserOpString) EndsWith(value string, sensitive bool) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v", value),
	}
}
