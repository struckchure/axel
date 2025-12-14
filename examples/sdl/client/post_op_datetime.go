package ax

import "fmt"

type PostOpDatetime struct {
	field string
}

func NewPostOpDatetime(field string) *PostOpDatetime {
	return &PostOpDatetime{field: field}
}

func (o *PostOpDatetime) Eq(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Eq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpDatetime) NotEq(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: NotEq,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpDatetime) Gt(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Gt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpDatetime) Gte(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Gte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpDatetime) Lt(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Lt,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}

func (o *PostOpDatetime) Lte(value string) *PostOp {
	return &PostOp{
		column:   o.field,
		operator: Lte,
		required: true,
		value:    fmt.Sprintf("%v", value),
	}
}
