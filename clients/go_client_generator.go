package clients

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/dave/jennifer/jen"
	"github.com/samber/lo"
	tree_sitter_axel "github.com/struckchure/axel/bindings/go"
	axel "github.com/struckchure/axel/core"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type GoClientGenerator struct {
	config *axel.MigrationConfig
	models []axel.Model
}

type goClientGeneratorValues struct {
	packageName string
	tableName   string
	tableFields []axel.Field
}

func (g *GoClientGenerator) Generate() error {
	asbtractModels := lo.Filter(g.models, func(model axel.Model, idx int) bool { return model.IsAbstract })
	concreteModels := lo.Filter(g.models, func(model axel.Model, idx int) bool { return !model.IsAbstract })

	for _, model := range concreteModels {
		if !lo.IsEmpty(model.Extends) {
			abstractModel, ok := lo.Find(asbtractModels, func(item axel.Model) bool { return item.Name == model.Extends })
			if !ok {
				return fmt.Errorf("abstract model %s not found", model.Extends)
			}

			model.Fields = append(abstractModel.Fields, model.Fields...)
		}

		values := goClientGeneratorValues{
			packageName: g.config.PackageName,
			tableName:   strings.ToLower(model.Name),
			tableFields: model.Fields,
		}

		err := g.generateConstants(values)
		if err != nil {
			return err
		}

		err = g.generateModel(model, values)
		if err != nil {
			return err
		}

		err = g.generateOperations(model, values)
		if err != nil {
			return err
		}

		err = g.generateFields(model, values)
		if err != nil {
			return err
		}

		err = g.generateQuery(model, values)
		if err != nil {
			return err
		}

		err = g.generateMutation(model, values)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *GoClientGenerator) generateConstants(values goClientGeneratorValues) error {
	f := jen.NewFile(values.packageName)

	f.Type().Id("Operator").String()
	f.Const().Defs(
		jen.Id("Eq").Op("Operator").Op("=").Lit("="),
		jen.Id("NotEq").Op("Operator").Op("=").Lit("!="),
		jen.Id("Gt").Op("Operator").Op("=").Lit(">"),
		jen.Id("Gte").Op("Operator").Op("=").Lit(">="),
		jen.Id("Lt").Op("Operator").Op("=").Lit("<"),
		jen.Id("Lte").Op("Operator").Op("=").Lit("<="),
		jen.Id("Like").Op("Operator").Op("=").Lit("LIKE"),
		jen.Id("Ilike").Op("Operator").Op("=").Lit("ILIKE"),
		jen.Id("In").Op("Operator").Op("=").Lit("IN"),
	)

	err := axel.WriteFile(path.Join(g.config.ClientDir, "op.go"), []byte(f.GoString()), 0644)
	if err != nil {
		return err
	}

	return nil
}

func (g *GoClientGenerator) generateModel(model axel.Model, values goClientGeneratorValues) error {
	f := jen.NewFile(values.packageName)

	fields := lo.Map(model.Fields, func(item axel.Field, idx int) jen.Code {
		_type, ok := goTypeMap[item.Type]
		if !ok {
			_type = item.Type
		}

		s := jen.Id(lo.PascalCase(item.Name))
		if item.IsMulti {
			s = s.Index()
		}

		s = s.Op(lo.Ternary(item.IsRequired, "", "*"))

		jenType := lo.Switch[string, *jen.Statement](_type).
			Case("uuid.UUID", jen.Qual("github.com/google/uuid", "UUID")).
			Case("time.Time", jen.Qual("time", "Time")).
			Default(jen.Id(_type))
		s = s.Custom(jen.Options{}, jenType)
		s = s.Tag(map[string]string{"db": lo.SnakeCase(item.Name), "json": lo.SnakeCase(item.Name)})

		return s
	})

	f.Type().
		Id(lo.PascalCase(values.tableName)).
		Struct(fields...)

	err := axel.WriteFile(path.Join(g.config.ClientDir, fmt.Sprintf("%s_model.go", strings.ToLower(model.Name))), []byte(f.GoString()), 0644)
	if err != nil {
		return err
	}

	return nil
}

func (g *GoClientGenerator) generateOperations(model axel.Model, values goClientGeneratorValues) error {
	f1 := jen.NewFile(values.packageName)

	f1.Type().Id(fmt.Sprintf("%sOp", model.Name)).Struct(
		jen.Id("column").String(),
		jen.Id("operator").Custom(jen.Options{}, jen.Id("Operator")),
		jen.Id("value").String(),
		jen.Id("required").Bool(),
	)

	err := axel.WriteFile(path.Join(g.config.ClientDir, strings.ToLower(fmt.Sprintf("%s_op.go", model.Name))), []byte(f1.GoString()), 0644)
	if err != nil {
		return err
	}

	for _, opType := range opTypes {
		f := jen.NewFile(values.packageName)

		structName := fmt.Sprintf("%sOp%s", model.Name, opType.name)
		generics := lo.Map(opType.generics, func(item string, idx int) jen.Code {
			return jen.Custom(jen.Options{}, jen.Id(item))
		})
		genericsArgs := strings.Join(lo.Map(opType.generics, func(item string, idx int) string { return strings.Split(item, " ")[0] }), ", ")

		struct_ := jen.Type().Id(structName)
		if len(generics) > 0 {
			struct_.Types(generics...)
		}
		struct_.Struct(jen.Id("field").String())
		f.Add(struct_)

		func_ := jen.Func().Id(fmt.Sprintf("New%s", structName))
		if len(generics) > 0 {
			func_.Types(generics...)
		}

		return_ := jen.
			Op("&").Id(structName)
		if len(generics) > 0 {
			return_.Index(jen.Id(genericsArgs))
		}
		return_.Values(jen.Dict{jen.Id("field"): jen.Id("field")})

		func_.Params(jen.Id("field").String()).
			Op("*").Add(
			lo.TernaryF(len(opType.generics) > 0,
				func() *jen.Statement { return jen.Id(structName).Index(jen.Id(genericsArgs)) },
				func() *jen.Statement { return jen.Id(structName) },
			),
		).
			Block(jen.Return(return_))
		func_.Line()
		f.Add(func_)

		for _, op := range opType.ops {
			f.Func().Add(
				lo.TernaryF(len(opType.generics) > 0,
					func() *jen.Statement {
						return jen.Params(jen.Id("o").Op("*").Id(structName).Index(jen.Id(genericsArgs)))
					},
					func() *jen.Statement { return jen.Params(jen.Id("o").Op("*").Id(structName)) },
				),
			).Id(op.name).
				Params(lo.Map(op.args, func(item string, idx int) jen.Code { return jen.Id(item) })...).
				Op("*").Id(fmt.Sprintf("%sOp", model.Name)).
				Block(
					jen.Return(
						jen.
							Op("&").
							Id(fmt.Sprintf("%sOp", model.Name)).
							Values(jen.Dict{
								jen.Id("column"):   jen.Id("o.field"),
								jen.Id("operator"): op.operator,
								jen.Id("value"):    op.value,
								jen.Id("required"): lo.Ternary(len(op.args) > 0, jen.True(), jen.False()),
							}),
					),
				)
			f.Line()
		}

		err := axel.WriteFile(path.Join(g.config.ClientDir, strings.ToLower(fmt.Sprintf("%s_op_%s.go", model.Name, opType.name))), []byte(f.GoString()), 0644)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *GoClientGenerator) generateFields(model axel.Model, values goClientGeneratorValues) error {
	f := jen.NewFile(values.packageName)

	f.Const().Defs(
		append(
			lo.Map(model.Fields, func(item axel.Field, idx int) jen.Code {
				return jen.
					Id(fmt.Sprintf("%sField%s", model.Name, lo.PascalCase(item.Name))).
					String().Op("=").
					Id(fmt.Sprintf("`\"%s\"`", lo.SnakeCase(item.Name)))
			}),
			jen.
				Id(fmt.Sprintf("%sTableName", model.Name)).
				String().Op("=").
				Id(fmt.Sprintf("`\"%s\"`", lo.SnakeCase(model.Name))),
		)...,
	)

	f.Var().Defs(
		lo.Map(model.Fields, func(item axel.Field, idx int) jen.Code {
			getOp := func(type_, name, field string, generics ...string) *jen.Statement {
				field = lo.PascalCase(field)

				if len(generics) > 0 {
					return jen.
						Id(fmt.Sprintf("New%sOp%s", name, type_)).
						Index(jen.Id(strings.Join(generics, ", "))).
						Call(jen.Id(fmt.Sprintf("%sField%s", name, field)))
				}

				return jen.
					Id(fmt.Sprintf("New%sOp%s", name, type_)).
					Call(jen.Id(fmt.Sprintf("%sField%s", name, field)))
			}

			defaultOp := getOp("String", model.Name, item.Name)

			return jen.
				Id(fmt.Sprintf("%s%s", model.Name, lo.PascalCase(item.Name))).
				Op("=").
				Add(
					lo.
						IfF(
							lo.Contains([]string{"int16", "int32", "int64", "float32", "float64"}, item.Type),
							func() *jen.Statement { return getOp("Number", model.Name, item.Name, item.Type) },
						).
						ElseIfF(
							item.Type == "datetime",
							func() *jen.Statement { return getOp("Datetime", model.Name, item.Name) },
						).
						ElseIfF(
							item.Type == "string",
							func() *jen.Statement { return defaultOp },
						).ElseF(
						func() *jen.Statement { return defaultOp },
					),
				)
		})...,
	)

	err := axel.WriteFile(path.Join(g.config.ClientDir, strings.ToLower(fmt.Sprintf("%s_fields.go", model.Name))), []byte(f.GoString()), 0644)
	if err != nil {
		return err
	}

	return nil
}

func (g *GoClientGenerator) generateQuery(model axel.Model, values goClientGeneratorValues) error {
	return nil
}

func (g *GoClientGenerator) generateMutation(model axel.Model, values goClientGeneratorValues) error {
	return nil
}

func NewGoClientGenerator(config *axel.MigrationConfig) (*GoClientGenerator, error) {
	// Read current schema file
	schemaCode, err := os.ReadFile(config.SchemaPath)
	if err != nil {
		return nil, err
	}

	// Setup tree-sitter parser
	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_axel.Language())

	// Parse current schema
	if err := parser.SetLanguage(lang); err != nil {
		return nil, err
	}

	tree := parser.Parse(schemaCode, nil)
	defer tree.Close()

	models := axel.ExtractModelsFromTree(tree.RootNode(), schemaCode)
	axel.ResolveOnTargetTypes(models)

	return &GoClientGenerator{models: models, config: config}, nil
}
