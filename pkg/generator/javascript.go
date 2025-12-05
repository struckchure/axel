package generator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/struckchure/axel/pkg/db"
)

// JavaScriptGenerator generates JavaScript code
type JavaScriptGenerator struct {
	outputDir string
}

// Generate generates JavaScript classes from schema
func (j *JavaScriptGenerator) Generate(schema *db.Schema) error {
	for _, table := range schema.Tables {
		content := j.generateClass(table)
		filename := filepath.Join(j.outputDir, "javascript", j.toKebabCase(table.Name)+".js")
		if err := writeFile(filename, content); err != nil {
			return err
		}
	}

	// Generate index.js
	indexContent := j.generateIndex(schema)
	indexFile := filepath.Join(j.outputDir, "javascript", "index.js")
	return writeFile(indexFile, indexContent)
}

func (j *JavaScriptGenerator) generateClass(table db.Table) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("/**\n * Represents the %s table\n */\n", table.Name))
	sb.WriteString(fmt.Sprintf("class %s {\n", j.toPascalCase(table.Name)))
	
	// Constructor
	sb.WriteString("  constructor(data = {}) {\n")
	for _, col := range table.Columns {
		sb.WriteString(fmt.Sprintf("    this.%s = data.%s;\n", 
			j.toCamelCase(col.Name), j.toCamelCase(col.Name)))
	}
	sb.WriteString("  }\n\n")

	// toJSON method
	sb.WriteString("  toJSON() {\n")
	sb.WriteString("    return {\n")
	for i, col := range table.Columns {
		comma := ","
		if i == len(table.Columns)-1 {
			comma = ""
		}
		sb.WriteString(fmt.Sprintf("      %s: this.%s%s\n", 
			j.toCamelCase(col.Name), j.toCamelCase(col.Name), comma))
	}
	sb.WriteString("    };\n")
	sb.WriteString("  }\n")

	sb.WriteString("}\n\n")
	sb.WriteString(fmt.Sprintf("module.exports = %s;\n", j.toPascalCase(table.Name)))
	return sb.String()
}

func (j *JavaScriptGenerator) generateIndex(schema *db.Schema) string {
	var sb strings.Builder

	sb.WriteString("// Generated models\n\n")
	for _, table := range schema.Tables {
		className := j.toPascalCase(table.Name)
		moduleName := j.toKebabCase(table.Name)
		sb.WriteString(fmt.Sprintf("const %s = require('./%s');\n", className, moduleName))
	}

	sb.WriteString("\nmodule.exports = {\n")
	for i, table := range schema.Tables {
		className := j.toPascalCase(table.Name)
		comma := ","
		if i == len(schema.Tables)-1 {
			comma = ""
		}
		sb.WriteString(fmt.Sprintf("  %s%s\n", className, comma))
	}
	sb.WriteString("};\n")

	return sb.String()
}

func (j *JavaScriptGenerator) toPascalCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, word := range words {
		words[i] = strings.Title(strings.ToLower(word))
	}
	return strings.Join(words, "")
}

func (j *JavaScriptGenerator) toCamelCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, word := range words {
		if i == 0 {
			words[i] = strings.ToLower(word)
		} else {
			words[i] = strings.Title(strings.ToLower(word))
		}
	}
	return strings.Join(words, "")
}

func (j *JavaScriptGenerator) toKebabCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == ' '
	})
	return strings.ToLower(strings.Join(words, "-"))
}
