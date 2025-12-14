package ax

import "fmt"

type UserOpBool struct {
	field string
}

func NewUserOpBool(field string) *UserOpBool {
	return &UserOpBool{field: field}
}

func (o *UserOpBool) Eq(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpBool) NotEq(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}
