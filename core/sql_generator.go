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

	// Assemble body rows in order: columns, foreign keys, then type-level constraints.
	rows := make([]string, 0, len(columns)+len(foreignKeys)+len(model.Constraints))
	rows = append(rows, columns...)
	rows = append(rows, foreignKeys...)
	for _, tc := range model.Constraints {
		if clause := typeConstraintClause(model.Name, tc); clause != "" {
			rows = append(rows, clause)
		}
	}

	for i, row := range rows {
		sql.WriteString("  " + row)
		if i < len(rows)-1 {
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

	// Generate indexes
	for _, stmt := range generateIndexes(model) {
		sql.WriteString("\n\n")
		sql.WriteString(stmt)
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

		// Body constraints on the link column (e.g. exclusive → UNIQUE).
		for _, constraint := range field.Constraints {
			switch constraint.Name {
			case "exclusive":
				parts = append(parts, namedConstraint(uniqueConstraintName(modelName, field.Name), "UNIQUE"))
			case "pk":
				parts = append(parts, namedConstraint(pkConstraintName(modelName), "PRIMARY KEY"))
			}
		}

		// Generate foreign key constraint
		refTable := formatIdentifier(field.Type)
		refColumn := formatIdentifier(field.OnTarget.Name)
		fkBody := fmt.Sprintf(`FOREIGN KEY (%s) REFERENCES %s(%s)`, colName, refTable, refColumn)

		if field.OnTarget.Name != "" {
			fkBody += " ON DELETE CASCADE"
		}
		foreignKey = namedConstraint(fkConstraintName(modelName, field.Name), fkBody)
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
				parts = append(parts, namedConstraint(uniqueConstraintName(modelName, field.Name), "UNIQUE"))
			case "pk":
				parts = append(parts, namedConstraint(pkConstraintName(modelName), "PRIMARY KEY"))
			}
		}

		// Length constraints → named CHECK clauses.
		parts = append(parts, lengthCheckClauses(modelName, field)...)

		// Enum-backed column → named membership CHECK.
		if clause := enumCheckClause(modelName, field); clause != "" {
			parts = append(parts, clause)
		}
	}

	return strings.Join(parts, " "), foreignKey
}

// enumCheckClause returns a named CHECK clause restricting an enum-backed column
// to its allowed values, e.g. `CONSTRAINT "chk_user_role_enum" CHECK ("role" IN (…))`.
func enumCheckClause(tableName string, field Field) string {
	if len(field.EnumValues) == 0 {
		return ""
	}
	colName := formatIdentifier(field.Name)
	body := fmt.Sprintf("CHECK (%s IN (%s))", colName, quotedEnumValues(field.EnumValues))
	return namedConstraint(enumConstraintName(tableName, field.Name), body)
}

// quotedEnumValues renders enum values as a comma-separated list of SQL string
// literals: 'Admin', 'Member', 'Guest'.
func quotedEnumValues(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + v + "'"
	}
	return strings.Join(quoted, ", ")
}

// lengthCheckClauses returns named CHECK clauses for min_length/max_length
// constraints on a string column, e.g.
// `CONSTRAINT "chk_user_email_min_length" CHECK (char_length("email") >= 6)`.
// Non-string columns are skipped since char_length only applies to text.
func lengthCheckClauses(tableName string, field Field) []string {
	if field.Type != "str" {
		return nil
	}

	colName := formatIdentifier(field.Name)

	var clauses []string
	for _, constraint := range field.Constraints {
		if len(constraint.Args) == 0 {
			continue
		}
		switch constraint.Name {
		case "min_length":
			body := fmt.Sprintf("CHECK (char_length(%s) >= %s)", colName, constraint.Args[0])
			clauses = append(clauses, namedConstraint(lengthConstraintName(tableName, field.Name, "min_length"), body))
		case "max_length":
			body := fmt.Sprintf("CHECK (char_length(%s) <= %s)", colName, constraint.Args[0])
			clauses = append(clauses, namedConstraint(lengthConstraintName(tableName, field.Name, "max_length"), body))
		}
	}

	return clauses
}

// indexName builds a deterministic index name from a table and its columns.
func indexName(tableName string, columns []string) string {
	parts := append([]string{"idx", lo.SnakeCase(tableName)}, columns...)
	return strings.Join(parts, "_")
}

// createIndexSQL builds a CREATE INDEX statement for the given table and columns.
func createIndexSQL(tableName string, columns []string) string {
	cols := make([]string, len(columns))
	for i, c := range columns {
		cols[i] = formatIdentifier(c)
	}
	return fmt.Sprintf(
		"CREATE INDEX IF NOT EXISTS %s ON %s (%s);",
		formatIdentifier(indexName(tableName, columns)),
		formatIdentifier(tableName),
		strings.Join(cols, ", "),
	)
}

// namedConstraint renders `CONSTRAINT "<name>" <body>`, quoting the name. Used so
// every constraint carries a deterministic name and can be dropped by a later
// migration, whether it was born in a CREATE TABLE or an ALTER TABLE.
func namedConstraint(name, body string) string {
	return fmt.Sprintf("CONSTRAINT %s %s", formatIdentifier(name), body)
}

// fkConstraintName names a single-column foreign key: fk_<table>_<col>.
func fkConstraintName(tableName, colName string) string {
	return fmt.Sprintf("fk_%s_%s", lo.SnakeCase(tableName), lo.SnakeCase(colName))
}

// uniqueConstraintName names a single-column unique constraint: uq_<table>_<col>.
// Consistent with the type-level uq_<table>_<cols…> from typeConstraintName.
func uniqueConstraintName(tableName, colName string) string {
	return fmt.Sprintf("uq_%s_%s", lo.SnakeCase(tableName), lo.SnakeCase(colName))
}

// pkConstraintName names a primary key: pk_<table>. Matches the type-level scheme.
func pkConstraintName(tableName string) string {
	return "pk_" + lo.SnakeCase(tableName)
}

// enumConstraintName names an enum-membership CHECK: chk_<table>_<col>_enum.
func enumConstraintName(tableName, colName string) string {
	return fmt.Sprintf("chk_%s_%s_enum", lo.SnakeCase(tableName), lo.SnakeCase(colName))
}

// typeConstraintName builds a deterministic name for a type-level constraint.
func typeConstraintName(tableName string, tc TypeConstraint) string {
	table := lo.SnakeCase(tableName)
	switch tc.Expression {
	case "exclusive":
		return strings.Join(append([]string{"uq", table}, tc.Columns...), "_")
	case "pk":
		return "pk_" + table
	case "min_length", "max_length":
		return strings.Join(append(append([]string{"chk", table}, tc.Columns...), tc.Expression), "_")
	default:
		return strings.Join(append([]string{"ck", table}, tc.Columns...), "_")
	}
}

// typeConstraintBody renders the constraint definition (without the leading
// CONSTRAINT <name>). Returns "" for unsupported/empty expressions.
func typeConstraintBody(tc TypeConstraint) string {
	if len(tc.Columns) == 0 {
		return ""
	}
	cols := make([]string, len(tc.Columns))
	for i, c := range tc.Columns {
		cols[i] = formatIdentifier(c)
	}

	switch tc.Expression {
	case "exclusive":
		return fmt.Sprintf("UNIQUE (%s)", strings.Join(cols, ", "))
	case "pk":
		return fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(cols, ", "))
	case "min_length", "max_length":
		if len(tc.Args) == 0 {
			return ""
		}
		op := ">="
		if tc.Expression == "max_length" {
			op = "<="
		}
		checks := make([]string, len(cols))
		for i, c := range cols {
			checks[i] = fmt.Sprintf("char_length(%s) %s %s", c, op, tc.Args[0])
		}
		return fmt.Sprintf("CHECK (%s)", strings.Join(checks, " AND "))
	default:
		return ""
	}
}

// typeConstraintClause returns an inline table constraint clause for use inside
// CREATE TABLE, e.g. `CONSTRAINT "uq_user_email_tenant_id" UNIQUE ("email", "tenant_id")`.
// Returns "" for unsupported expressions.
func typeConstraintClause(tableName string, tc TypeConstraint) string {
	body := typeConstraintBody(tc)
	if body == "" {
		return ""
	}
	return fmt.Sprintf("CONSTRAINT %s %s", formatIdentifier(typeConstraintName(tableName, tc)), body)
}

// generateIndexes returns CREATE INDEX statements for all of a model's indexes.
func generateIndexes(model Model) []string {
	var statements []string
	for _, idx := range model.Indexes {
		if len(idx.Columns) == 0 {
			continue
		}
		statements = append(statements, createIndexSQL(model.Name, idx.Columns))
	}
	return statements
}

func generateJunctionTable(modelName string, field Field) string {
	junction := fmt.Sprintf("%s_%s", lo.SnakeCase(modelName), lo.SnakeCase(field.Name))
	tableName := formatIdentifier(junction)
	refTable := formatIdentifier(field.Type)
	modelTable := formatIdentifier(modelName)

	pk := namedConstraint(pkConstraintName(junction),
		fmt.Sprintf("PRIMARY KEY (%s, %s)", formatIdentifier(modelName), formatIdentifier(field.Type)))
	fkModel := namedConstraint(fkConstraintName(junction, modelName),
		fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE CASCADE", formatIdentifier(modelName), modelTable, "id"))
	fkTarget := namedConstraint(fkConstraintName(junction, field.Type),
		fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE CASCADE", formatIdentifier(field.Type), refTable, "id"))

	return fmt.Sprintf(`CREATE TABLE %s (
  %s UUID NOT NULL,
  %s UUID NOT NULL,
  %s,
  %s,
  %s
);`,
		tableName,
		formatIdentifier(modelName), formatIdentifier(field.Type),
		pk,       // TODO: reference fields assumed to be "id"
		fkModel,  // TODO: should be the reference field
		fkTarget, // TODO: should be the reference field
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
