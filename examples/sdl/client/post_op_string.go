package ax

import (
	"fmt"
	lo "github.com/samber/lo"
)

type PostOpString struct {
	Field string
}

func NewPostOpString(field string) *PostOpString {
	return &PostOpString{Field: field}
}

func (o *PostOpString) Eq(value string) *PostOp {
	return &PostOp{
		column:   o.Field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpString) NotEq(value string) *PostOp {
	return &PostOp{
		column:   o.Field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpString) Contains(value string, sensitive bool) *PostOp {
	return &PostOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v%%", value),
	}
}

func (o *PostOpString) StartsWith(value string, sensitive bool) *PostOp {
	return &PostOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%v%%", value),
	}
}

func (o *PostOpString) EndsWith(value string, sensitive bool) *PostOp {
	return &PostOp{
		column:   o.Field,
		operator: lo.Ternary(sensitive, Like, Ilike),
		required: true,
		value:    fmt.Sprintf("%%%v", value),
	}
}
