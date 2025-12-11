package clients

import "github.com/dave/jennifer/jen"

type op struct {
	name     string
	args     []string
	operator jen.Code
	value    jen.Code
}

type opType struct {
	name        string
	hasGenerics bool
	ops         []op
}

var opTypes = []opType{
	{
		name:        "String",
		hasGenerics: false,
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
				name: "Contains",
				operator: jen.Qual("github.com/samber/lo", "Ternary").Call(
					jen.Id("sensitive"),
					jen.Id("Like"),
					jen.Id("Ilike"),
				),
				value: jen.Qual("fmt", "Sprintf").Call(
					jen.Lit("%%%v%%"),
					jen.Id("value"),
				),
				args: []string{"value string", "sensitive bool"},
			},
			{
				name: "StartsWith",
				operator: jen.Qual("github.com/samber/lo", "Ternary").Call(
					jen.Id("sensitive"),
					jen.Id("Like"),
					jen.Id("Ilike"),
				),
				value: jen.Qual("fmt", "Sprintf").Call(
					jen.Lit("%v%%"),
					jen.Id("value"),
				),
				args: []string{"value string", "sensitive bool"},
			},
			{
				name: "EndsWith",
				operator: jen.Qual("github.com/samber/lo", "Ternary").Call(
					jen.Id("sensitive"),
					jen.Id("Like"),
					jen.Id("Ilike"),
				),
				value: jen.Qual("fmt", "Sprintf").Call(
					jen.Lit("%%%v"),
					jen.Id("value"),
				),
				args: []string{"value string", "sensitive bool"},
			},
		},
	},
}
