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
	currentSchema := SchemaIRToModels(ir)

	// Get last schema snapshot.
	lastSchema, err := g.manager.GetLastSchema()
	if err != nil {
		return fmt.Errorf("failed to get last schema: %w", err)
	}

	// Detect changes.
	changes := DiffSchemas(lastSchema, currentSchema)
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
	if err := g.manager.CreateMigrationDir(version, name, currentSchema, upSQL, downSQL); err != nil {
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
