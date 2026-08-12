package axel

import (
	"fmt"
	"os"

	"github.com/struckchure/axel/core/asl"
)

// MigrationGenerator handles generating migration files from schema changes.
type MigrationGenerator struct {
	manager *MigrationManager
}

// NewMigrationGenerator creates a new migration generator.
func NewMigrationGenerator(manager *MigrationManager) *MigrationGenerator {
	return &MigrationGenerator{manager: manager}
}

// GenerateMigration generates a new migration based on schema changes.
func (g *MigrationGenerator) GenerateMigration(name string) error {
	schemaCode, err := os.ReadFile(g.manager.config.SchemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	// Parse with the ASL parser.
	src, err := asl.Parse(schemaCode)
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Resolve to SchemaIR.
	resolver := &asl.Resolver{}
	ir, err := resolver.Resolve(src)
	if err != nil {
		return fmt.Errorf("failed to resolve schema: %w", err)
	}

	// Validate.
	if errs := asl.Validate(ir); len(errs) > 0 {
		return fmt.Errorf("schema validation errors:\n%v", errs)
	}

	// Convert to legacy []Model for the existing migration SQL generator.
	// OnTarget.Type is already resolved inside SchemaIRToModels.
	currentSchema, err := SchemaIRToModels(ir)
	if err != nil {
		return fmt.Errorf("failed to lower schema: %w", err)
	}

	// Lower rewrites/triggers/functions to flat SQL.
	currentFns, currentTrigs, err := SchemaIRToFunctionsAndTriggers(ir)
	if err != nil {
		return fmt.Errorf("failed to lower triggers/functions: %w", err)
	}
	currentExts := SchemaIRToExtensions(ir)
	currentPols, err := SchemaIRToPolicies(ir)
	if err != nil {
		return fmt.Errorf("failed to lower policies: %w", err)
	}

	// Get last snapshot (schema + functions + triggers + extensions).
	lastSchema, err := g.manager.GetLastSchema()
	if err != nil {
		return fmt.Errorf("failed to get last schema: %w", err)
	}
	lastFns, lastTrigs, err := g.manager.GetLastFunctionsAndTriggers()
	if err != nil {
		return fmt.Errorf("failed to get last functions/triggers: %w", err)
	}
	lastExts, err := g.manager.GetLastExtensions()
	if err != nil {
		return fmt.Errorf("failed to get last extensions: %w", err)
	}
	lastPols, err := g.manager.GetLastPolicies()
	if err != nil {
		return fmt.Errorf("failed to get last policies: %w", err)
	}

	// Detect changes: extensions first, then tables, then functions, then
	// triggers (ordering is enforced in GenerateMigrationSQL — extensions and
	// AddModel sort ahead of everything else).
	changes := DiffExtensions(lastExts, currentExts)
	changes = append(changes, DiffSchemas(lastSchema, currentSchema)...)
	changes = append(changes, DiffFunctions(lastFns, currentFns)...)
	changes = append(changes, DiffTriggers(lastTrigs, currentTrigs)...)
	changes = append(changes, DiffPolicies(lastPols, currentPols)...)
	if len(changes) == 0 {
		return fmt.Errorf("no schema changes detected")
	}

	// Generate SQL.
	upSQL, downSQL := GenerateMigrationSQL(changes, lastSchema, currentSchema)

	// Get next version.
	version, err := g.manager.GetNextVersion()
	if err != nil {
		return fmt.Errorf("failed to get next version: %w", err)
	}

	// Write migration files.
	if err := g.manager.CreateMigrationDir(version, name, currentSchema, currentFns, currentTrigs, currentExts, currentPols, upSQL, downSQL); err != nil {
		return fmt.Errorf("failed to create migration: %w", err)
	}

	fmt.Printf("Migration %s created successfully\n", version)
	fmt.Printf("  Location: %s/%s\n", g.manager.config.MigrationsDir, version)
	fmt.Printf("  Changes: %d\n", len(changes))

	for _, change := range changes {
		fmt.Printf("    - %s\n", change.Description)
	}

	return nil
}
