package generator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/struckchure/axel/pkg/db"
)

// GoGenerator generates Go code
type GoGenerator struct {
	outputDir string
	pkg       string
}

// Generate generates Go structs from schema
func (g *GoGenerator) Generate(schema *db.Schema) error {
	if g.pkg == "" {
		g.pkg = "models"
	}

	for _, table := range schema.Tables {
		content := g.generateStruct(table)
		filename := filepath.Join(g.outputDir, "go", g.toSnakeCase(table.Name)+".go")
		if err := writeFile(filename, content); err != nil {
			return err
		}
	}

	return nil
}

func (g *GoGenerator) generateStruct(table db.Table) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("package %s\n\n", g.pkg))
	sb.WriteString(fmt.Sprintf("// %s represents the %s table\n", g.toPascalCase(table.Name), table.Name))
	sb.WriteString(fmt.Sprintf("type %s struct {\n", g.toPascalCase(table.Name)))

	for _, col := range table.Columns {
		goType := g.mapTypeToGo(col.Type, col.Nullable)
		sb.WriteString(fmt.Sprintf("\t%s %s `json:\"%s\" db:\"%s\"`\n",
			g.toPascalCase(col.Name), goType, g.toSnakeCase(col.Name), col.Name))
	}

	sb.WriteString("}\n")
	return sb.String()
}

func (g *GoGenerator) mapTypeToGo(dbType string, nullable bool) string {
	var goType string

	dbType = strings.ToLower(dbType)

	// Map common database types to Go types
	switch {
	case strings.Contains(dbType, "bigint"):
		goType = "int64"
	case strings.Contains(dbType, "smallint"):
		goType = "int16"
	case strings.Contains(dbType, "int"):
		goType = "int"
	case strings.Contains(dbType, "boolean") || strings.Contains(dbType, "bool"):
		goType = "bool"
	case strings.Contains(dbType, "varchar") || strings.Contains(dbType, "text") || strings.Contains(dbType, "char"):
		goType = "string"
	case strings.Contains(dbType, "timestamp") || strings.Contains(dbType, "datetime"):
		goType = "time.Time"
	case strings.Contains(dbType, "date"):
		goType = "time.Time"
	case strings.Contains(dbType, "float") || strings.Contains(dbType, "double") || strings.Contains(dbType, "decimal"):
		goType = "float64"
	case strings.Contains(dbType, "json"):
		goType = "json.RawMessage"
	default:
		goType = "interface{}"
	}

	if nullable && goType != "interface{}" {
		goType = "*" + goType
	}

	return goType
}

func (g *GoGenerator) toPascalCase(s string) string {
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

func (g *GoGenerator) toSnakeCase(s string) string {
	// If already contains underscores, just lowercase it
	if strings.Contains(s, "_") {
		return strings.ToLower(s)
	}
	// Otherwise, return as-is (assuming it's already in correct format)
	return strings.ToLower(s)
}
