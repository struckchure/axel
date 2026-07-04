// Package tests holds black-box integration tests that exercise Axel through
// its public APIs (asl.Parse/Resolve, core.SchemaIRToModels/DiffSchemas/
// GenerateMigrationSQL, codegen.FromSchemaIR/Walk).
package tests

import (
	"testing"

	axel "github.com/struckchure/axel/core"
	"github.com/struckchure/axel/core/asl"
)

// parseSchema parses and resolves an .asl source, failing the test on error.
func parseSchema(t *testing.T, schema string) *asl.SchemaIR {
	t.Helper()
	src, err := asl.Parse([]byte(schema))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&asl.Resolver{}).Resolve(src)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return ir
}

// resolveErr parses (which must succeed) then resolves, returning the resolve error.
func resolveErr(t *testing.T, schema string) error {
	t.Helper()
	src, err := asl.Parse([]byte(schema))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = (&asl.Resolver{}).Resolve(src)
	return err
}

// parseToModels resolves a schema into the legacy []Model form.
func parseToModels(t *testing.T, schema string) []axel.Model {
	t.Helper()
	return axel.SchemaIRToModels(parseSchema(t, schema))
}

// genUp returns the up-migration SQL for a schema built from an empty baseline.
func genUp(t *testing.T, schema string) string {
	t.Helper()
	models := parseToModels(t, schema)
	up, _ := axel.GenerateMigrationSQL(axel.DiffSchemas(nil, models), nil, models)
	return up
}

// genMigration returns the up/down SQL for the diff between two schemas.
func genMigration(t *testing.T, oldSchema, newSchema string) (up, down string) {
	t.Helper()
	oldModels := parseToModels(t, oldSchema)
	newModels := parseToModels(t, newSchema)
	return axel.GenerateMigrationSQL(axel.DiffSchemas(oldModels, newModels), oldModels, newModels)
}
