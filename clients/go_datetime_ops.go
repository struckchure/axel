package clients

var goDatetimeOps = opType{
	name:     "Datetime",
	generics: []string{},
	ops:      append(goNumberOps.ops, []op{}...),
}
