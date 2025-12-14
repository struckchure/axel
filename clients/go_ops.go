package clients

import "github.com/dave/jennifer/jen"

type op struct {
	name     string
	args     []string
	operator jen.Code
	value    jen.Code
}

type opType struct {
	name     string
	generics []string
	ops      []op
}

var opTypes = []opType{
	goBoolOps,
	goStringOps,
	goNumberOps,
	goDatetimeOps,
}
