package generator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/struckchure/axel/pkg/db"
)

// PythonGenerator generates Python code
type PythonGenerator struct {
	outputDir string
}

// Generate generates Python classes from schema
func (p *PythonGenerator) Generate(schema *db.Schema) error {
	for _, table := range schema.Tables {
		content := p.generateClass(table)
		filename := filepath.Join(p.outputDir, "python", p.toSnakeCase(table.Name)+".py")
		if err := writeFile(filename, content); err != nil {
			return err
		}
	}

	// Generate __init__.py
	initContent := p.generateInit(schema)
	initFile := filepath.Join(p.outputDir, "python", "__init__.py")
	return writeFile(initFile, initContent)
}

func (p *PythonGenerator) generateClass(table db.Table) string {
	var sb strings.Builder

	sb.WriteString("from dataclasses import dataclass\n")
	sb.WriteString("from typing import Optional, Any\n")
	sb.WriteString("from datetime import datetime\n\n")
	sb.WriteString(fmt.Sprintf("@dataclass\n"))
	sb.WriteString(fmt.Sprintf("class %s:\n", p.toPascalCase(table.Name)))
	sb.WriteString(fmt.Sprintf("    \"\"\"Represents the %s table\"\"\"\n", table.Name))

	for _, col := range table.Columns {
		pyType := p.mapTypeToPython(col.Type, col.Nullable)
		sb.WriteString(fmt.Sprintf("    %s: %s\n", p.toSnakeCase(col.Name), pyType))
	}

	return sb.String()
}

func (p *PythonGenerator) generateInit(schema *db.Schema) string {
	var sb strings.Builder

	sb.WriteString("# Generated models\n\n")
	for _, table := range schema.Tables {
		className := p.toPascalCase(table.Name)
		moduleName := p.toSnakeCase(table.Name)
		sb.WriteString(fmt.Sprintf("from .%s import %s\n", moduleName, className))
	}

	sb.WriteString("\n__all__ = [\n")
	for i, table := range schema.Tables {
		className := p.toPascalCase(table.Name)
		if i < len(schema.Tables)-1 {
			sb.WriteString(fmt.Sprintf("    '%s',\n", className))
		} else {
			sb.WriteString(fmt.Sprintf("    '%s'\n", className))
		}
	}
	sb.WriteString("]\n")

	return sb.String()
}

func (p *PythonGenerator) mapTypeToPython(dbType string, nullable bool) string {
	var pyType string

	dbType = strings.ToLower(dbType)

	switch {
	case strings.Contains(dbType, "int"):
		pyType = "int"
	case strings.Contains(dbType, "boolean") || strings.Contains(dbType, "bool"):
		pyType = "bool"
	case strings.Contains(dbType, "varchar") || strings.Contains(dbType, "text") || strings.Contains(dbType, "char"):
		pyType = "str"
	case strings.Contains(dbType, "timestamp") || strings.Contains(dbType, "datetime") || strings.Contains(dbType, "date"):
		pyType = "datetime"
	case strings.Contains(dbType, "float") || strings.Contains(dbType, "double") || strings.Contains(dbType, "decimal") || strings.Contains(dbType, "real"):
		pyType = "float"
	default:
		pyType = "Any"
	}

	if nullable {
		pyType = "Optional[" + pyType + "]"
	}

	return pyType
}

func (p *PythonGenerator) toPascalCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, "")
}

func (p *PythonGenerator) toSnakeCase(s string) string {
	// If already contains underscores, just lowercase it
	if strings.Contains(s, "_") {
		return strings.ToLower(s)
	}
	// Otherwise, return as-is (assuming it's already in correct format)
	return strings.ToLower(s)
}
