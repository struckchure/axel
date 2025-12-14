package ax

import (
	"fmt"
	lo "github.com/samber/lo"
)

type PostOpString struct {
	field string
}

func NewPostOpString(field string) *PostOpString {
	return &PostOpString{field: field}
}

func (o *PostOpString) Eq(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpString) NotEq(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpString) Contains(value string, sensitive bool) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v%%", value),
	}
}

func (o *PostOpString) StartsWith(value string, sensitive bool) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%v%%", value),
	}
}

func (o *PostOpString) EndsWith(value string, sensitive bool) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v", value),
	}
}
