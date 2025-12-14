package ax

import "fmt"

type PostOpBool struct {
	field string
}

func NewPostOpBool(field string) *PostOpBool {
	return &PostOpBool{field: field}
}

func (o *PostOpBool) Eq(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpBool) NotEq(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}
