package ax

import "fmt"

type UserOpDatetime struct {
	field string
}

func NewUserOpDatetime(field string) *UserOpDatetime {
	return &UserOpDatetime{field: field}
}

func (o *UserOpDatetime) Eq(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpDatetime) NotEq(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpDatetime) Gt(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Gt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpDatetime) Gte(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Gte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpDatetime) Lt(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Lt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *UserOpDatetime) Lte(value string) *UserOp {
	return &UserOp{
		column:   o.field,
		operator: Lte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}
