package axel

import (
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"
)

func generateTable(model Model, abstractModels map[string]Model) string {
	var sql strings.Builder

	sql.WriteString(fmt.Sprintf(`CREATE TABLE %s (`, formatIdentifier(model.Name)))
	sql.WriteString("\n")

	var columns []string
	var foreignKeys []string

	// Inherit fields from parent
	if model.Extends != "" {
		if parent, ok := abstractModels[model.Extends]; ok {
			for _, field := range parent.Fields {
				col, fk := generateColumn(field, model.Name)
				columns = append(columns, col)
				if fk != "" {
					foreignKeys = append(foreignKeys, fk)
				}
			}
		}
	}

	// Add model's own fields
	for _, field := range model.Fields {
		// Skip multi fields - they need junction tables
		if field.IsMulti {
			continue
		}

		col, fk := generateColumn(field, model.Name)
		columns = append(columns, col)
		if fk != "" {
			foreignKeys = append(foreignKeys, fk)
		}
	}

	// Add all columns
	for i, col := range columns {
		sql.WriteString("  " + col)
		if i < len(columns)-1 || len(foreignKeys) > 0 {
			sql.WriteString(",")
		}
		sql.WriteString("\n")
	}

	// Add foreign keys
	for i, fk := range foreignKeys {
		sql.WriteString("  " + fk)
		if i < len(foreignKeys)-1 {
			sql.WriteString(",")
		}
		sql.WriteString("\n")
	}

	sql.WriteString(");")

	// Generate junction tables for multi fields
	for _, field := range model.Fields {
		if field.IsMulti {
			sql.WriteString("\n\n")
			sql.WriteString(generateJunctionTable(model.Name, field))
		}
	}

	return sql.String()
}

func generateColumn(field Field, modelName string) (string, string) {
	colName := formatIdentifier(field.Name)
	sqlType := mapType(field.Type)

	var parts []string
	parts = append(parts, colName)
	parts = append(parts, sqlType)

	// Check if it's a link (foreign key)
	isLink := !isBuiltinType(field.Type)
	var foreignKey string

	if isLink {
		// Foreign key column
		parts = []string{colName, mapType(field.OnTarget.Type)}

		if field.IsRequired {
			parts = append(parts, "NOT NULL")
		}

		// Generate foreign key constraint
		refTable := formatIdentifier(field.Type)
		refColumn := formatIdentifier(field.OnTarget.Name)
		foreignKey = fmt.Sprintf(`FOREIGN KEY (%s) REFERENCES %s(%s)`, colName, refTable, refColumn)

		if field.OnTarget.Name != "" {
			foreignKey += " ON DELETE CASCADE"
		}
	} else {
		// Regular column
		if field.IsRequired {
			parts = append(parts, "NOT NULL")
		}

		if field.Default != "" {
			defaultVal := mapDefault(field.Default, sqlType)
			parts = append(parts, "DEFAULT "+defaultVal)
		}

		// Add constraints
		for _, constraint := range field.Constraints {
			switch constraint.Name {
			case "exclusive":
				parts = append(parts, "UNIQUE")
			case "pk":
				parts = append(parts, "PRIMARY KEY")
			}
		}
	}

	return strings.Join(parts, " "), foreignKey
}

func generateJunctionTable(modelName string, field Field) string {
	tableName := formatIdentifier(fmt.Sprintf("%s_%s", lo.SnakeCase(modelName), lo.SnakeCase(field.Name)))
	refTable := formatIdentifier(field.Type)
	modelTable := formatIdentifier(modelName)

	return fmt.Sprintf(`CREATE TABLE %s (
  %s UUID NOT NULL,
  %s UUID NOT NULL,
  PRIMARY KEY (%s, %s),
  FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE CASCADE,
  FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE CASCADE
);`,
		tableName,
		formatIdentifier(modelName), formatIdentifier(field.Type),
		formatIdentifier(modelName), formatIdentifier(field.Type),
		formatIdentifier(modelName), modelTable, "id", // TODO: should be the reference field
		formatIdentifier(field.Type), refTable, "id", // TODO: should be the reference field
	)
}

func mapType(axelType string) string {
	typeMap := map[string]string{
		"str":      "TEXT",
		"int16":    "SMALLINT",
		"int32":    "INTEGER",
		"int64":    "BIGINT",
		"float32":  "REAL",
		"float64":  "DOUBLE PRECISION",
		"bool":     "BOOLEAN",
		"uuid":     "UUID",
		"datetime": "TIMESTAMP",
		"json":     "JSONB",
		"bytes":    "BYTEA",
	}

	if sqlType, ok := typeMap[axelType]; ok {
		return sqlType
	}

	return "UUID" // Assume it's a link
}

func mapDefault(defaultVal, sqlType string) string {
	// Check if it's a function call
	if !strings.HasPrefix(defaultVal, "@func(") {
		// It's a regular expression/literal, return as-is
		return defaultVal
	}

	// Extract function name from @func(function_name) pattern
	funcName := ""
	if strings.HasSuffix(defaultVal, ")") {
		funcName = strings.TrimSuffix(strings.TrimPrefix(defaultVal, "@func("), ")")
	}

	// Map EdgeDB functions to SQL equivalents
	switch funcName {
	case "now":
		return "CURRENT_TIMESTAMP"
	default:
		return fmt.Sprintf("%s()", funcName)
	}
}

func isBuiltinType(typeName string) bool {
	builtins := []string{
		"str", "int16", "int32", "int64",
		"float32", "float64", "bool", "uuid",
		"datetime", "json", "bytes",
	}

	return slices.Contains(builtins, typeName)
}
