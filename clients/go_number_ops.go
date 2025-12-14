package clients

import "github.com/dave/jennifer/jen"

var goNumberOps = opType{
	name:     "Number",
	generics: []string{"T any"},
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
		{
			name:     "Gt",
			operator: jen.Id("Gt"),
			value: jen.Qual("fmt", "Sprintf").Call(
				jen.Lit("%v"),
				jen.Id("value"),
			),
			args: []string{"value string"},
		},
		{
			name:     "Gte",
			operator: jen.Id("Gte"),
			value: jen.Qual("fmt", "Sprintf").Call(
				jen.Lit("%v"),
				jen.Id("value"),
			),
			args: []string{"value string"},
		},
		{
			name:     "Lt",
			operator: jen.Id("Lt"),
			value: jen.Qual("fmt", "Sprintf").Call(
				jen.Lit("%v"),
				jen.Id("value"),
			),
			args: []string{"value string"},
		},
		{
			name:     "Lte",
			operator: jen.Id("Lte"),
			value: jen.Qual("fmt", "Sprintf").Call(
				jen.Lit("%v"),
				jen.Id("value"),
			),
			args: []string{"value string"},
		},
	},
}
