package axel

import (
	"fmt"
	"slices"
	"strings"

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

			// Check for index changes
			indexChanges := diffIndexes(oldModel, newModel)
			changes = append(changes, indexChanges...)

			// Check for type-level constraint changes
			constraintChanges := diffTypeConstraints(oldModel, newModel)
			changes = append(changes, constraintChanges...)
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

// diffIndexes compares indexes between two versions of a model, keyed by the
// deterministic index name (column order is significant).
func diffIndexes(oldModel, newModel Model) []SchemaChange {
	var changes []SchemaChange

	oldIndexes := make(map[string]Index)
	newIndexes := make(map[string]Index)

	for _, idx := range oldModel.Indexes {
		oldIndexes[indexName(oldModel.Name, idx.Columns)] = idx
	}
	for _, idx := range newModel.Indexes {
		newIndexes[indexName(newModel.Name, idx.Columns)] = idx
	}

	// Added indexes.
	for key, idx := range newIndexes {
		if _, exists := oldIndexes[key]; !exists {
			changes = append(changes, SchemaChange{
				Type:        AddIndex,
				ModelName:   newModel.Name,
				NewValue:    idx,
				Description: fmt.Sprintf("Add index on '%s' (%s)", lo.SnakeCase(newModel.Name), strings.Join(idx.Columns, ", ")),
			})
		}
	}

	// Removed indexes.
	for key, idx := range oldIndexes {
		if _, exists := newIndexes[key]; !exists {
			changes = append(changes, SchemaChange{
				Type:        DropIndex,
				ModelName:   newModel.Name,
				OldValue:    idx,
				Description: fmt.Sprintf("Drop index on '%s' (%s)", lo.SnakeCase(newModel.Name), strings.Join(idx.Columns, ", ")),
			})
		}
	}

	return changes
}

// diffTypeConstraints compares type-level constraints between two versions of a
// model, keyed by the deterministic constraint name.
func diffTypeConstraints(oldModel, newModel Model) []SchemaChange {
	var changes []SchemaChange

	oldCons := make(map[string]TypeConstraint)
	newCons := make(map[string]TypeConstraint)

	for _, tc := range oldModel.Constraints {
		oldCons[typeConstraintName(oldModel.Name, tc)] = tc
	}
	for _, tc := range newModel.Constraints {
		newCons[typeConstraintName(newModel.Name, tc)] = tc
	}

	// Added constraints.
	for key, tc := range newCons {
		if _, exists := oldCons[key]; !exists {
			changes = append(changes, SchemaChange{
				Type:        AddConstraint,
				ModelName:   newModel.Name,
				NewValue:    tc,
				Description: fmt.Sprintf("Add constraint '%s' on '%s'", key, lo.SnakeCase(newModel.Name)),
			})
		}
	}

	// Removed constraints.
	for key, tc := range oldCons {
		if _, exists := newCons[key]; !exists {
			changes = append(changes, SchemaChange{
				Type:        DropConstraint,
				ModelName:   newModel.Name,
				OldValue:    tc,
				Description: fmt.Sprintf("Drop constraint '%s' on '%s'", key, lo.SnakeCase(newModel.Name)),
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

	// Enum backing
	if f1.EnumType != f2.EnumType {
		return false
	}
	if !slices.Equal(f1.EnumValues, f2.EnumValues) {
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
