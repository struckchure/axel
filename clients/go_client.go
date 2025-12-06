package clients

import axel "github.com/struckchure/axel/core"

type GoClientGenerator struct{}

type GenerateOptions struct {
	Models []axel.Model
}

func (g *GoClientGenerator) Generate(opts GenerateOptions) error {
	return nil
}

func NewGoClientGenerator() *GoClientGenerator {
	return &GoClientGenerator{}
}
