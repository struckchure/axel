package axel

import (
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"
)

// GenerateMigrationSQL generates both up and down SQL from schema changes
func GenerateMigrationSQL(changes []SchemaChange, oldSchema, newSchema []Model) (upSQL, downSQL string) {
	var upStatements []string
	var downStatements []string

	// Extensions are no longer auto-injected. Declare them explicitly in the
	// schema, e.g. `use extension 'pgcrypto';` (required for gen_random_uuid()).

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

	// Extensions must be created before any table/function that may depend on them,
	// so they lead; Functions (declared top-level / rewrites) must be created
	// before tables so that default expressions and check constraints referencing
	// them resolve cleanly; AddModel follows (dependency-sorted); everything else
	// keeps its order.
	var extChanges []SchemaChange
	var fnChanges []SchemaChange
	var addModelChanges []SchemaChange
	var otherChanges []SchemaChange
	var runOnceCalls []string
	var junctionUpStatements []string

	for _, change := range changes {
		switch change.Type {
		case AddExtension, DropExtension:
			extChanges = append(extChanges, change)
		case AddFunction, ModifyFunction:
			fnChanges = append(fnChanges, change)
		case AddModel:
			addModelChanges = append(addModelChanges, change)
		default:
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

	// Merge back: extensions first, then functions, then AddModel (sorted), then others. Because
	// down statements are reversed for rollback, processing extensions first also
	// makes their DROPs run last on the way down.
	sortedChanges := make([]SchemaChange, 0, len(extChanges)+len(fnChanges)+len(addModelChanges)+len(otherChanges))
	sortedChanges = append(sortedChanges, extChanges...)
	sortedChanges = append(sortedChanges, fnChanges...)
	sortedChanges = append(sortedChanges, addModelChanges...)
	sortedChanges = append(sortedChanges, otherChanges...)

	// Process changes in order
	for _, change := range sortedChanges {
		switch change.Type {
		case AddModel:
			model := change.NewValue.(Model)
			tableName := lo.SnakeCase(model.Name)

			up := generateTable(model, newAbstract)
			down := fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE;", tableName)

			// Handle junction tables for multi link fields
			for _, field := range model.Fields {
				if field.IsLink && field.IsMulti {
					junctionTable := fmt.Sprintf("%s_%s", tableName, lo.SnakeCase(field.Name))
					junctionUpStatements = append(junctionUpStatements, generateJunctionTable(model.Name, field))
					down = fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE;\n", junctionTable) + down
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
				if field.IsLink && field.IsMulti {
					junctionTable := fmt.Sprintf("%s_%s", tableName, lo.SnakeCase(field.Name))
					up = fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE;\n", junctionTable) + up
				}
			}

			upStatements = append(upStatements, up)
			downStatements = append(downStatements, down)

		case AddField:
			field := change.NewValue.(Field)
			tableName := lo.SnakeCase(change.ModelName)

			if field.IsLink && field.IsMulti {
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

			if field.IsLink && field.IsMulti {
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

		case AddIndex:
			index := change.NewValue.(Index)
			up := createIndexSQL(change.ModelName, index)
			down := fmt.Sprintf("DROP INDEX IF EXISTS %s;", formatIdentifier(indexName(change.ModelName, index)))

			upStatements = append(upStatements, up)
			downStatements = append(downStatements, down)

		case DropIndex:
			index := change.OldValue.(Index)
			up := fmt.Sprintf("DROP INDEX IF EXISTS %s;", formatIdentifier(indexName(change.ModelName, index)))
			down := createIndexSQL(change.ModelName, index)

			upStatements = append(upStatements, up)
			downStatements = append(downStatements, down)

		case AddConstraint:
			tc := change.NewValue.(TypeConstraint)
			up := typeConstraintAddSQL(change.ModelName, tc)
			if up != "" {
				upStatements = append(upStatements, up)
				downStatements = append(downStatements, typeConstraintDropSQL(change.ModelName, tc))
			}

		case DropConstraint:
			tc := change.OldValue.(TypeConstraint)
			down := typeConstraintAddSQL(change.ModelName, tc)
			if down != "" {
				upStatements = append(upStatements, typeConstraintDropSQL(change.ModelName, tc))
				downStatements = append(downStatements, down)
			}

		case AddExtension:
			ext := change.NewValue.(Extension)
			upStatements = append(upStatements, ext.CreateSQL)
			downStatements = append(downStatements, ext.DropSQL)

		case DropExtension:
			ext := change.OldValue.(Extension)
			upStatements = append(upStatements, ext.DropSQL)
			downStatements = append(downStatements, ext.CreateSQL)

		case AddFunction:
			fn := change.NewValue.(Function)
			upStatements = append(upStatements, fn.CreateSQL)
			// @for setup functions run once when first created, after tables exist.
			if fn.RunOnce {
				runOnceCalls = append(runOnceCalls, fmt.Sprintf("SELECT %q();", fn.Name))
			}
			downStatements = append(downStatements, fn.DropSQL)

		case DropFunction:
			fn := change.OldValue.(Function)
			upStatements = append(upStatements, fn.DropSQL)
			downStatements = append(downStatements, fn.CreateSQL)

		case ModifyFunction:
			// CREATE OR REPLACE swaps the body in place; down restores the old one.
			oldFn := change.OldValue.(Function)
			newFn := change.NewValue.(Function)
			upStatements = append(upStatements, newFn.CreateSQL)
			downStatements = append(downStatements, oldFn.CreateSQL)

		case AddTrigger:
			trg := change.NewValue.(Trigger)
			upStatements = append(upStatements, trg.CreateSQL)
			downStatements = append(downStatements, trg.DropSQL)

		case DropTrigger:
			trg := change.OldValue.(Trigger)
			upStatements = append(upStatements, trg.DropSQL)
			downStatements = append(downStatements, trg.CreateSQL)

		case ModifyTrigger:
			// Triggers can't be replaced in place across PG versions → drop + create.
			oldTrg := change.OldValue.(Trigger)
			newTrg := change.NewValue.(Trigger)
			upStatements = append(upStatements, oldTrg.DropSQL+"\n"+newTrg.CreateSQL)
			downStatements = append(downStatements, newTrg.DropSQL+"\n"+oldTrg.CreateSQL)

		case AddPolicy:
			pol := change.NewValue.(Policy)
			upStatements = append(upStatements, pol.CreateSQL)
			downStatements = append(downStatements, pol.DropSQL)

		case DropPolicy:
			pol := change.OldValue.(Policy)
			upStatements = append(upStatements, pol.DropSQL)
			downStatements = append(downStatements, pol.CreateSQL)

		case ModifyPolicy:
			// Policies can't be altered in place → drop + create.
			oldPol := change.OldValue.(Policy)
			newPol := change.NewValue.(Policy)
			upStatements = append(upStatements, oldPol.DropSQL+"\n"+newPol.CreateSQL)
			downStatements = append(downStatements, newPol.DropSQL+"\n"+oldPol.CreateSQL)
		}
	}

	// Junction tables must be created after all base tables exist so both foreign
	// keys resolve cleanly.
	if len(junctionUpStatements) > 0 {
		upStatements = append(upStatements, junctionUpStatements...)
	}

	// Reverse down statements for rollback
	for i := len(downStatements) - 1; i >= 0; i-- {
		if downSQL != "" {
			downSQL += "\n\n"
		}
		downSQL += downStatements[i]
	}

	if len(runOnceCalls) > 0 {
		upStatements = append(upStatements, runOnceCalls...)
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
			// Only consider non-multi foreign keys as dependencies. Skip
			// self-references: a table can always reference itself once created,
			// and treating it as a dependency would drop it from the sort below.
			if field.IsLink && !field.IsMulti && field.Type != model.Name {
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

	// A genuine dependency cycle aborts a DFS branch early (visit returns false),
	// leaving some models unvisited. Append them in their original order so no
	// table is dropped from the migration — foreign keys are emitted as separate
	// ALTER TABLE statements, so they still resolve once every table exists.
	for _, model := range models {
		if !visited[model.Name] {
			visited[model.Name] = true
			sorted = append(sorted, model)
		}
	}

	return sorted
}

// generateAddColumn generates ALTER TABLE ADD COLUMN statement
func generateAddColumn(tableName string, field Field) string {
	colName := lo.SnakeCase(field.Name)
	sqlType := mapType(field.Type)
	if field.IsMulti {
		if field.Type != "json" && field.Type != "jsonb" {
			sqlType += "[]"
		}
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("ALTER TABLE \"%s\" ADD COLUMN %s", tableName, colName))

	if field.IsLink {
		parts = append(parts, mapType(field.OnTarget.Type))
	} else {
		parts = append(parts, sqlType)
	}

	if field.IsRequired {
		parts = append(parts, "NOT NULL")
	}

	if !field.IsLink && field.Default != "" {
		defaultVal := mapDefault(field.Default, sqlType)
		parts = append(parts, "DEFAULT "+defaultVal)
	}

	// Add constraints
	for _, constraint := range field.Constraints {
		switch constraint.Name {
		case "exclusive":
			parts = append(parts, namedConstraint(uniqueConstraintName(tableName, field.Name), "UNIQUE"))
		}
	}

	// Length + enum constraints → named CHECK clauses.
	if !field.IsLink {
		parts = append(parts, lengthCheckClauses(tableName, field)...)
		if clause := enumCheckClause(tableName, field); clause != "" {
			parts = append(parts, clause)
		}
	}

	stmt := strings.Join(parts, " ") + ";"

	// Add foreign key if it's a link
	if field.IsLink {
		refTable := lo.SnakeCase(field.Type)
		refColumn := lo.SnakeCase(field.OnTarget.Name)
		stmt += fmt.Sprintf("\nALTER TABLE \"%s\" ADD %s;",
			tableName,
			namedConstraint(fkConstraintName(tableName, field.Name),
				fmt.Sprintf("FOREIGN KEY (%s) REFERENCES \"%s\"(%s) ON DELETE CASCADE", colName, refTable, refColumn)))
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

	// Length constraint changes (min_length / max_length).
	oldChecks := lengthConstraintMap(oldField)
	newChecks := lengthConstraintMap(newField)

	for _, kind := range []string{"min_length", "max_length"} {
		oldArg, hadOld := oldChecks[kind]
		newArg, hasNew := newChecks[kind]
		if hadOld == hasNew && oldArg == newArg {
			continue
		}

		name := formatIdentifier(lengthConstraintName(tableName, colName, kind))
		dropStmt := fmt.Sprintf("ALTER TABLE \"%s\" DROP CONSTRAINT IF EXISTS %s;", tableName, name)

		if hasNew {
			// Added or changed: ensure the old (if any) is gone, then add the new.
			addStmt := fmt.Sprintf(
				"ALTER TABLE \"%s\" ADD CONSTRAINT %s CHECK (char_length(%s) %s %s);",
				tableName, name, formatIdentifier(newField.Name), lengthCheckOp(kind), newArg,
			)
			upParts = append(upParts, addStmt)
			downParts = append(downParts, dropStmt)
		} else {
			// Removed.
			upParts = append(upParts, dropStmt)
			if hadOld {
				downParts = append(downParts, fmt.Sprintf(
					"ALTER TABLE \"%s\" ADD CONSTRAINT %s CHECK (char_length(%s) %s %s);",
					tableName, name, formatIdentifier(oldField.Name), lengthCheckOp(kind), oldArg,
				))
			}
		}
	}

	// Enum membership change (add / remove / change of allowed values).
	if !slices.Equal(oldField.EnumValues, newField.EnumValues) {
		name := formatIdentifier(enumConstraintName(tableName, colName))
		dropStmt := fmt.Sprintf("ALTER TABLE \"%s\" DROP CONSTRAINT IF EXISTS %s;", tableName, name)
		addNew := fmt.Sprintf("ALTER TABLE \"%s\" ADD CONSTRAINT %s CHECK (%s IN (%s));",
			tableName, name, formatIdentifier(newField.Name), quotedEnumValues(newField.EnumValues))
		addOld := fmt.Sprintf("ALTER TABLE \"%s\" ADD CONSTRAINT %s CHECK (%s IN (%s));",
			tableName, name, formatIdentifier(oldField.Name), quotedEnumValues(oldField.EnumValues))

		if len(newField.EnumValues) > 0 {
			// Replace any prior enum check with the new one.
			if len(oldField.EnumValues) > 0 {
				upParts = append(upParts, dropStmt)
			}
			upParts = append(upParts, addNew)
			downParts = append(downParts, dropStmt)
			if len(oldField.EnumValues) > 0 {
				downParts = append(downParts, addOld)
			}
		} else {
			// Enum backing removed.
			upParts = append(upParts, dropStmt)
			if len(oldField.EnumValues) > 0 {
				downParts = append(downParts, addOld)
			}
		}
	}

	upSQL = strings.Join(upParts, "\n")
	downSQL = strings.Join(downParts, "\n")

	return upSQL, downSQL
}

// typeConstraintAddSQL builds an ALTER TABLE ADD CONSTRAINT statement for a
// type-level constraint. Returns "" for unsupported expressions.
func typeConstraintAddSQL(tableName string, tc TypeConstraint) string {
	body := typeConstraintBody(tc)
	if body == "" {
		return ""
	}
	return fmt.Sprintf("ALTER TABLE \"%s\" ADD CONSTRAINT %s %s;",
		lo.SnakeCase(tableName), formatIdentifier(typeConstraintName(tableName, tc)), body)
}

// typeConstraintDropSQL builds an ALTER TABLE DROP CONSTRAINT statement.
func typeConstraintDropSQL(tableName string, tc TypeConstraint) string {
	return fmt.Sprintf("ALTER TABLE \"%s\" DROP CONSTRAINT IF EXISTS %s;",
		lo.SnakeCase(tableName), formatIdentifier(typeConstraintName(tableName, tc)))
}

// lengthConstraintMap returns the min_length/max_length constraint arguments for
// a string field, keyed by constraint name. Non-string fields yield an empty map.
func lengthConstraintMap(field Field) map[string]string {
	checks := make(map[string]string)
	if field.Type != "str" {
		return checks
	}
	for _, c := range field.Constraints {
		if len(c.Args) == 0 {
			continue
		}
		if c.Name == "min_length" || c.Name == "max_length" {
			checks[c.Name] = c.Args[0]
		}
	}
	return checks
}

// lengthConstraintName builds the deterministic name for a named length CHECK
// constraint: chk_<table>_<col>_<kind>. Snakes its inputs so raw or already-snake
// names both yield the same result.
func lengthConstraintName(tableName, colName, kind string) string {
	return fmt.Sprintf("chk_%s_%s_%s", lo.SnakeCase(tableName), lo.SnakeCase(colName), kind)
}

// lengthCheckOp returns the comparison operator for a length constraint kind.
func lengthCheckOp(kind string) string {
	if kind == "max_length" {
		return "<="
	}
	return ">="
}

// generateJunctionTableForField creates junction table SQL for a multi field
func generateJunctionTableForField(modelName string, field Field) string {
	return generateJunctionTable(modelName, field)
}
