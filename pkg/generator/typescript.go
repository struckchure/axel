package generator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/struckchure/axel/pkg/db"
)

// TypeScriptGenerator generates TypeScript code
type TypeScriptGenerator struct {
	outputDir string
}

// Generate generates TypeScript interfaces from schema
func (t *TypeScriptGenerator) Generate(schema *db.Schema) error {
	for _, table := range schema.Tables {
		content := t.generateInterface(table)
		filename := filepath.Join(t.outputDir, "typescript", t.toKebabCase(table.Name)+".ts")
		if err := writeFile(filename, content); err != nil {
			return err
		}
	}

	// Generate index.ts
	indexContent := t.generateIndex(schema)
	indexFile := filepath.Join(t.outputDir, "typescript", "index.ts")
	return writeFile(indexFile, indexContent)
}

func (t *TypeScriptGenerator) generateInterface(table db.Table) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("/**\n * Represents the %s table\n */\n", table.Name))
	sb.WriteString(fmt.Sprintf("export interface %s {\n", t.toPascalCase(table.Name)))

	for _, col := range table.Columns {
		tsType := t.mapTypeToTypeScript(col.Type, col.Nullable)
		optional := ""
		if col.Nullable {
			optional = "?"
		}
		sb.WriteString(fmt.Sprintf("  %s%s: %s;\n", t.toCamelCase(col.Name), optional, tsType))
	}

	sb.WriteString("}\n")
	return sb.String()
}

func (t *TypeScriptGenerator) generateIndex(schema *db.Schema) string {
	var sb strings.Builder

	sb.WriteString("// Generated models\n\n")
	for _, table := range schema.Tables {
		className := t.toPascalCase(table.Name)
		moduleName := t.toKebabCase(table.Name)
		sb.WriteString(fmt.Sprintf("export { %s } from './%s';\n", className, moduleName))
	}

	return sb.String()
}

func (t *TypeScriptGenerator) mapTypeToTypeScript(dbType string, nullable bool) string {
	var tsType string

	dbType = strings.ToLower(dbType)

	switch {
	case strings.Contains(dbType, "int") || strings.Contains(dbType, "float") || strings.Contains(dbType, "double") || strings.Contains(dbType, "decimal") || strings.Contains(dbType, "real"):
		tsType = "number"
	case strings.Contains(dbType, "boolean") || strings.Contains(dbType, "bool"):
		tsType = "boolean"
	case strings.Contains(dbType, "varchar") || strings.Contains(dbType, "text") || strings.Contains(dbType, "char"):
		tsType = "string"
	case strings.Contains(dbType, "timestamp") || strings.Contains(dbType, "datetime") || strings.Contains(dbType, "date"):
		tsType = "Date"
	case strings.Contains(dbType, "json"):
		tsType = "any"
	default:
		tsType = "unknown"
	}

	if nullable {
		tsType = tsType + " | null"
	}

	return tsType
}

func (t *TypeScriptGenerator) toPascalCase(s string) string {
	return toPascalCase(s)
}

func (t *TypeScriptGenerator) toCamelCase(s string) string {
	return toCamelCase(s)
}

func (t *TypeScriptGenerator) toKebabCase(s string) string {
	return toKebabCase(s)
}
