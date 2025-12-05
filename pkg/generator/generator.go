package generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/struckchure/axel/pkg/config"
	"github.com/struckchure/axel/pkg/db"
)

// Generator handles code generation
type Generator struct {
	config *config.Config
}

// NewGenerator creates a new code generator
func NewGenerator(cfg *config.Config) *Generator {
	return &Generator{config: cfg}
}

// Generate generates code based on the schema
func (g *Generator) Generate(schema *db.Schema) error {
	if err := os.MkdirAll(g.config.Output.Directory, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for _, genCfg := range g.config.Generators {
		switch genCfg.Language {
		case "go":
			if err := g.generateGo(schema, genCfg); err != nil {
				return fmt.Errorf("failed to generate Go code: %w", err)
			}
		case "python":
			if err := g.generatePython(schema, genCfg); err != nil {
				return fmt.Errorf("failed to generate Python code: %w", err)
			}
		case "typescript":
			if err := g.generateTypeScript(schema, genCfg); err != nil {
				return fmt.Errorf("failed to generate TypeScript code: %w", err)
			}
		case "javascript":
			if err := g.generateJavaScript(schema, genCfg); err != nil {
				return fmt.Errorf("failed to generate JavaScript code: %w", err)
			}
		default:
			return fmt.Errorf("unsupported language: %s", genCfg.Language)
		}
	}

	return nil
}

func (g *Generator) generateGo(schema *db.Schema, genCfg config.GeneratorConfig) error {
	gen := &GoGenerator{
		outputDir: g.config.Output.Directory,
		pkg:       genCfg.Options["package"],
	}
	return gen.Generate(schema)
}

func (g *Generator) generatePython(schema *db.Schema, genCfg config.GeneratorConfig) error {
	gen := &PythonGenerator{
		outputDir: g.config.Output.Directory,
	}
	return gen.Generate(schema)
}

func (g *Generator) generateTypeScript(schema *db.Schema, genCfg config.GeneratorConfig) error {
	gen := &TypeScriptGenerator{
		outputDir: g.config.Output.Directory,
	}
	return gen.Generate(schema)
}

func (g *Generator) generateJavaScript(schema *db.Schema, genCfg config.GeneratorConfig) error {
	gen := &JavaScriptGenerator{
		outputDir: g.config.Output.Directory,
	}
	return gen.Generate(schema)
}

// Helper function to write file
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}
