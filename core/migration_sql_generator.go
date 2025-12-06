package axel

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
)

// GenerateMigrationSQL generates both up and down SQL from schema changes
func GenerateMigrationSQL(changes []SchemaChange, oldSchema, newSchema []Model) (upSQL, downSQL string) {
	var upStatements []string
	var downStatements []string

	// Add pgcrypto extension as the first statement (only if there are changes)
	if len(changes) > 0 {
		upStatements = append(upStatements, "CREATE EXTENSION IF NOT EXISTS \"pgcrypto\";")
		downStatements = append(downStatements, "DROP EXTENSION IF EXISTS \"pgcrypto\";")
	}

	// Build abstract model maps for lookups
	oldAbstract := make(map[string]Model)
	newAbstract := make(map[string]Model)

	for _, model := range oldSchema {
		if model.IsAbstract {
			oldAbstract[model.Name] = model
		}
	}

	for _, model := range newSchema {
		if model.IsAbstract {
			newAbstract[model.Name] = model
		}
	}

	// Separate AddModel changes from others and sort them by dependencies
	var addModelChanges []SchemaChange
	var otherChanges []SchemaChange

	for _, change := range changes {
		if change.Type == AddModel {
			addModelChanges = append(addModelChanges, change)
		} else {
			otherChanges = append(otherChanges, change)
		}
	}

	// Sort AddModel changes by dependencies
	if len(addModelChanges) > 0 {
		models := make([]Model, len(addModelChanges))
		for i, change := range addModelChanges {
			models[i] = change.NewValue.(Model)
		}
		sortedModels := topologicalSort(models)

		// Create new sorted addModelChanges
		addModelChanges = make([]SchemaChange, len(sortedModels))
		for i, model := range sortedModels {
			addModelChanges[i] = SchemaChange{
				Type:     AddModel,
				NewValue: model,
			}
		}
	}

	// Merge back: AddModel changes first (sorted), then others
	sortedChanges := append(addModelChanges, otherChanges...)

	// Process changes in order
	for _, change := range sortedChanges {
		switch change.Type {
		case AddModel:
			model := change.NewValue.(Model)
			tableName := lo.SnakeCase(model.Name)

			up := generateTable(model, newAbstract)
			down := fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE;", tableName)

			// Handle junction tables for multi fields
			for _, field := range model.Fields {
				if field.IsMulti {
					junctionTable := fmt.Sprintf("%s_%s", tableName, lo.SnakeCase(field.Name))
					down += fmt.Sprintf("\nDROP TABLE IF EXISTS \"%s\" CASCADE;", junctionTable)
				}
			}

			upStatements = append(upStatements, up)
			downStatements = append(downStatements, down)

		case DropModel:
			model := change.OldValue.(Model)
			tableName := lo.SnakeCase(model.Name)

			up := fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE;", tableName)
			down := generateTable(model, oldAbstract)

			// Handle junction tables
			for _, field := range model.Fields {
				if field.IsMulti {
					junctionTable := fmt.Sprintf("%s_%s", tableName, lo.SnakeCase(field.Name))
					up = fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE;\n", junctionTable) + up
				}
			}

			upStatements = append(upStatements, up)
			downStatements = append(downStatements, down)

		case AddField:
			field := change.NewValue.(Field)
			tableName := lo.SnakeCase(change.ModelName)

			if field.IsMulti {
				// Create junction table
				junctionSQL := generateJunctionTableForField(change.ModelName, field)
				junctionTable := fmt.Sprintf("%s_%s", tableName, lo.SnakeCase(field.Name))

				upStatements = append(upStatements, junctionSQL)
				downStatements = append(downStatements, fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE;", junctionTable))
			} else {
				up := generateAddColumn(tableName, field)
				down := fmt.Sprintf("ALTER TABLE \"%s\" DROP COLUMN IF EXISTS %s;", tableName, lo.SnakeCase(field.Name))

				upStatements = append(upStatements, up)
				downStatements = append(downStatements, down)
			}

		case DropField:
			field := change.OldValue.(Field)
			tableName := lo.SnakeCase(change.ModelName)

			if field.IsMulti {
				junctionTable := fmt.Sprintf("%s_%s", tableName, lo.SnakeCase(field.Name))
				upStatements = append(upStatements, fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE;", junctionTable))
				downStatements = append(downStatements, generateJunctionTableForField(change.ModelName, field))
			} else {
				up := fmt.Sprintf("ALTER TABLE \"%s\" DROP COLUMN IF EXISTS %s;", tableName, lo.SnakeCase(field.Name))
				down := generateAddColumn(tableName, field)

				upStatements = append(upStatements, up)
				downStatements = append(downStatements, down)
			}

		case ModifyField:
			oldField := change.OldValue.(Field)
			newField := change.NewValue.(Field)
			tableName := lo.SnakeCase(change.ModelName)

			// Generate ALTER statements for the modification
			up, down := generateModifyColumn(tableName, oldField, newField)
			if up != "" {
				upStatements = append(upStatements, up)
				downStatements = append(downStatements, down)
			}
		}
	}

	// Reverse down statements for rollback
	for i := len(downStatements) - 1; i >= 0; i-- {
		if downSQL != "" {
			downSQL += "\n\n"
		}
		downSQL += downStatements[i]
	}

	upSQL = strings.Join(upStatements, "\n\n")

	return upSQL, downSQL
}

// topologicalSort sorts models so dependencies are created first
func topologicalSort(models []Model) []Model {
	// Build dependency graph
	deps := make(map[string][]string)
	modelMap := make(map[string]Model)

	for _, model := range models {
		modelMap[model.Name] = model
		deps[model.Name] = []string{}

		for _, field := range model.Fields {
			// Only consider non-multi foreign keys as dependencies
			if !isBuiltinType(field.Type) && !field.IsMulti {
				deps[model.Name] = append(deps[model.Name], field.Type)
			}
		}
	}

	// Topological sort using DFS
	var sorted []Model
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var visit func(string) bool
	visit = func(name string) bool {
		if visited[name] {
			return true
		}
		if visiting[name] {
			// Circular dependency detected
			return false
		}

		visiting[name] = true

		for _, dep := range deps[name] {
			if _, exists := modelMap[dep]; exists {
				if !visit(dep) {
					return false
				}
			}
		}

		visiting[name] = false
		visited[name] = true

		if model, exists := modelMap[name]; exists {
			sorted = append(sorted, model)
		}

		return true
	}

	for _, model := range models {
		if !visited[model.Name] {
			visit(model.Name)
		}
	}

	return sorted
}

// generateAddColumn generates ALTER TABLE ADD COLUMN statement
func generateAddColumn(tableName string, field Field) string {
	colName := lo.SnakeCase(field.Name)
	sqlType := mapType(field.Type)

	var parts []string
	parts = append(parts, fmt.Sprintf("ALTER TABLE \"%s\" ADD COLUMN %s", tableName, colName))

	isLink := !isBuiltinType(field.Type)

	if isLink {
		parts = append(parts, mapType(field.OnTarget.Type))
	} else {
		parts = append(parts, sqlType)
	}

	if field.IsRequired {
		parts = append(parts, "NOT NULL")
	}

	if !isLink && field.Default != "" {
		defaultVal := mapDefault(field.Default, sqlType)
		parts = append(parts, "DEFAULT "+defaultVal)
	}

	// Add constraints
	for _, constraint := range field.Constraints {
		switch constraint.Name {
		case "exclusive":
			parts = append(parts, "UNIQUE")
		}
	}

	stmt := strings.Join(parts, " ") + ";"

	// Add foreign key if it's a link
	if isLink {
		refTable := lo.SnakeCase(field.Type)
		refColumn := lo.SnakeCase(field.OnTarget.Name)
		stmt += fmt.Sprintf("\nALTER TABLE \"%s\" ADD CONSTRAINT fk_%s_%s FOREIGN KEY (%s) REFERENCES \"%s\"(%s) ON DELETE CASCADE;",
			tableName, tableName, colName, colName, refTable, refColumn)
	}

	return stmt
}

// generateModifyColumn generates ALTER statements for field modifications
func generateModifyColumn(tableName string, oldField, newField Field) (upSQL, downSQL string) {
	colName := lo.SnakeCase(newField.Name)

	var upParts []string
	var downParts []string

	// Type change
	if oldField.Type != newField.Type {
		oldType := mapType(oldField.Type)
		newType := mapType(newField.Type)

		upParts = append(upParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s TYPE %s;", tableName, colName, newType))
		downParts = append(downParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s TYPE %s;", tableName, colName, oldType))
	}

	// Required constraint change
	if oldField.IsRequired != newField.IsRequired {
		if newField.IsRequired {
			upParts = append(upParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s SET NOT NULL;", tableName, colName))
			downParts = append(downParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s DROP NOT NULL;", tableName, colName))
		} else {
			upParts = append(upParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s DROP NOT NULL;", tableName, colName))
			downParts = append(downParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s SET NOT NULL;", tableName, colName))
		}
	}

	// Default value change
	if oldField.Default != newField.Default {
		sqlType := mapType(newField.Type)

		if newField.Default != "" {
			newDefault := mapDefault(newField.Default, sqlType)
			upParts = append(upParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s SET DEFAULT %s;", tableName, colName, newDefault))
		} else {
			upParts = append(upParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s DROP DEFAULT;", tableName, colName))
		}

		if oldField.Default != "" {
			oldDefault := mapDefault(oldField.Default, mapType(oldField.Type))
			downParts = append(downParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s SET DEFAULT %s;", tableName, colName, oldDefault))
		} else {
			downParts = append(downParts, fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN %s DROP DEFAULT;", tableName, colName))
		}
	}

	upSQL = strings.Join(upParts, "\n")
	downSQL = strings.Join(downParts, "\n")

	return upSQL, downSQL
}

// generateJunctionTableForField creates junction table SQL for a multi field
func generateJunctionTableForField(modelName string, field Field) string {
	return generateJunctionTable(modelName, field)
}
