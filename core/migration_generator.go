package axel

import (
	"fmt"
	"os"

	"github.com/samber/lo"

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
	// SchemaPath may be a file, a directory, or a glob matching several .asl
	// files; asl.LoadIR reads, parses and merges them all.
	ir, _, err := asl.LoadIR(g.manager.config.SchemaPath)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
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

	warnings := backfillWarnings(changes)

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

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	return nil
}

// backfillWarnings flags columns that become NOT NULL on a table that already
// exists — newly added, or flipped from optional to required.
// Such a column is created nullable and set NOT NULL in a second statement (see
// generateAddColumn); on a table with rows that statement fails until the
// migration is edited to backfill a value, so say so at generation time rather
// than letting `axel up` be the messenger.
func backfillWarnings(changes []SchemaChange) []string {
	var warnings []string
	for _, change := range changes {
		if change.Type != AddField && change.Type != ModifyField {
			continue
		}
		field, ok := change.NewValue.(Field)
		if !ok || !field.IsRequired || field.IsMulti {
			continue
		}
		if change.Type == ModifyField {
			// Only a nullable → required flip needs a backfill; anything else about
			// an already-required column is safe.
			old, ok := change.OldValue.(Field)
			if !ok || old.IsRequired {
				continue
			}
		}
		if !field.IsLink && field.Default != "" {
			continue // the default backfills existing rows
		}
		warnings = append(warnings, fmt.Sprintf(
			"%s.%s is required with no default: existing rows must be backfilled in the migration before its SET NOT NULL succeeds",
			lo.SnakeCase(change.ModelName), lo.SnakeCase(field.Name)))
	}
	return warnings
}
