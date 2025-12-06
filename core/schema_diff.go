package axel

import (
	"fmt"

	"github.com/samber/lo"
)

// DiffSchemas compares two schemas and returns the changes
func DiffSchemas(oldSchema, newSchema []Model) []SchemaChange {
	var changes []SchemaChange

	// Create maps for quick lookup
	oldModels := make(map[string]Model)
	newModels := make(map[string]Model)

	for _, model := range oldSchema {
		if !model.IsAbstract {
			oldModels[model.Name] = model
		}
	}

	for _, model := range newSchema {
		if !model.IsAbstract {
			newModels[model.Name] = model
		}
	}

	// Check for added and modified models
	for name, newModel := range newModels {
		if oldModel, exists := oldModels[name]; exists {
			// Model exists, check for field changes
			fieldChanges := diffFields(oldModel, newModel)
			changes = append(changes, fieldChanges...)
		} else {
			// New model
			changes = append(changes, SchemaChange{
				Type:        AddModel,
				ModelName:   name,
				NewValue:    newModel,
				Description: fmt.Sprintf("Add table '%s'", lo.SnakeCase(name)),
			})
		}
	}

	// Check for removed models
	for name, oldModel := range oldModels {
		if _, exists := newModels[name]; !exists {
			changes = append(changes, SchemaChange{
				Type:        DropModel,
				ModelName:   name,
				OldValue:    oldModel,
				Description: fmt.Sprintf("Drop table '%s'", lo.SnakeCase(name)),
			})
		}
	}

	return changes
}

// diffFields compares fields between two versions of a model
func diffFields(oldModel, newModel Model) []SchemaChange {
	var changes []SchemaChange

	// Create maps for field lookup
	oldFields := make(map[string]Field)
	newFields := make(map[string]Field)

	// Include inherited fields
	allOldFields := getAllFields(oldModel, make(map[string]Model))
	allNewFields := getAllFields(newModel, make(map[string]Model))

	for _, field := range allOldFields {
		oldFields[field.Name] = field
	}

	for _, field := range allNewFields {
		newFields[field.Name] = field
	}

	// Check for added and modified fields
	for name, newField := range newFields {
		if oldField, exists := oldFields[name]; exists {
			// Field exists, check if modified
			if !fieldsEqual(oldField, newField) {
				changes = append(changes, SchemaChange{
					Type:        ModifyField,
					ModelName:   newModel.Name,
					FieldName:   name,
					OldValue:    oldField,
					NewValue:    newField,
					Description: fmt.Sprintf("Modify column '%s.%s'", lo.SnakeCase(newModel.Name), lo.SnakeCase(name)),
				})
			}
		} else {
			// New field
			changes = append(changes, SchemaChange{
				Type:        AddField,
				ModelName:   newModel.Name,
				FieldName:   name,
				NewValue:    newField,
				Description: fmt.Sprintf("Add column '%s.%s'", lo.SnakeCase(newModel.Name), lo.SnakeCase(name)),
			})
		}
	}

	// Check for removed fields
	for name, oldField := range oldFields {
		if _, exists := newFields[name]; !exists {
			changes = append(changes, SchemaChange{
				Type:        DropField,
				ModelName:   newModel.Name,
				FieldName:   name,
				OldValue:    oldField,
				Description: fmt.Sprintf("Drop column '%s.%s'", lo.SnakeCase(newModel.Name), lo.SnakeCase(name)),
			})
		}
	}

	return changes
}

// getAllFields returns all fields including inherited ones
func getAllFields(model Model, abstractModels map[string]Model) []Field {
	var allFields []Field

	// Add inherited fields
	if model.Extends != "" {
		if parent, exists := abstractModels[model.Extends]; exists {
			allFields = append(allFields, getAllFields(parent, abstractModels)...)
		}
	}

	// Add own fields
	allFields = append(allFields, model.Fields...)

	return allFields
}

// fieldsEqual checks if two fields are equivalent
func fieldsEqual(f1, f2 Field) bool {
	if f1.Type != f2.Type {
		return false
	}

	if f1.IsRequired != f2.IsRequired {
		return false
	}

	if f1.IsMulti != f2.IsMulti {
		return false
	}

	if f1.Default != f2.Default {
		return false
	}

	if f1.OnTarget.Name != f2.OnTarget.Name {
		return false
	}

	if f1.OnTarget.Type != f2.OnTarget.Type {
		return false
	}

	// Check constraints
	if len(f1.Constraints) != len(f2.Constraints) {
		return false
	}

	// Create constraint maps for comparison
	c1Map := make(map[string]bool)
	c2Map := make(map[string]bool)

	for _, c := range f1.Constraints {
		c1Map[constraintKey(c)] = true
	}

	for _, c := range f2.Constraints {
		c2Map[constraintKey(c)] = true
	}

	for key := range c1Map {
		if !c2Map[key] {
			return false
		}
	}

	return true
}

// constraintKey generates a unique key for a constraint
func constraintKey(c Constraint) string {
	key := c.Name
	for _, arg := range c.Args {
		key += ":" + arg
	}
	return key
}
