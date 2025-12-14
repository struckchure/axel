package clients

import "github.com/dave/jennifer/jen"

var goBoolOps = opType{
	name:     "Bool",
	generics: []string{},
	ops: []op{
		{
			name:     "Eq",
			operator: jen.Id("Eq"),
			value: jen.Qual("fmt", "Sprintf").Call(
				jen.Lit("%v"),
				jen.Id("value"),
			),
			args: []string{"value string"},
		},
		{
			name:     "NotEq",
			operator: jen.Id("NotEq"),
			value: jen.Qual("fmt", "Sprintf").Call(
				jen.Lit("%v"),
				jen.Id("value"),
			),
			args: []string{"value string"},
		},
	},
}
