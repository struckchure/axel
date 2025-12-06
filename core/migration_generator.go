package axel

import (
	"fmt"
	"os"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// MigrationGenerator handles generating migration files from schema changes
type MigrationGenerator struct {
	manager *MigrationManager
	parser  *tree_sitter.Parser
	lang    *tree_sitter.Language
}

// NewMigrationGenerator creates a new migration generator
func NewMigrationGenerator(manager *MigrationManager, parser *tree_sitter.Parser, lang *tree_sitter.Language) *MigrationGenerator {
	return &MigrationGenerator{
		manager: manager,
		parser:  parser,
		lang:    lang,
	}
}

// GenerateMigration generates a new migration based on schema changes
func (g *MigrationGenerator) GenerateMigration(name string) error {
	// Read current schema file
	schemaCode, err := os.ReadFile(g.manager.config.SchemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	// Parse current schema
	if err := g.parser.SetLanguage(g.lang); err != nil {
		return fmt.Errorf("failed to set language: %w", err)
	}

	tree := g.parser.Parse(schemaCode, nil)
	defer tree.Close()

	currentSchema := ExtractModelsFromTree(tree.RootNode(), schemaCode)
	ResolveOnTargetTypes(currentSchema)

	// Get last schema
	lastSchema, err := g.manager.GetLastSchema()
	if err != nil {
		return fmt.Errorf("failed to get last schema: %w", err)
	}

	// Detect changes
	changes := DiffSchemas(lastSchema, currentSchema)

	if len(changes) == 0 {
		return fmt.Errorf("no schema changes detected")
	}

	// Generate SQL
	upSQL, downSQL := GenerateMigrationSQL(changes, lastSchema, currentSchema)

	// Get next version
	version, err := g.manager.GetNextVersion()
	if err != nil {
		return fmt.Errorf("failed to get next version: %w", err)
	}

	// Create migration directory
	if err := g.manager.CreateMigrationDir(version, name, currentSchema, upSQL, downSQL); err != nil {
		return fmt.Errorf("failed to create migration: %w", err)
	}

	fmt.Printf("Migration %s created successfully\n", version)
	fmt.Printf("  Location: %s/%s\n", g.manager.config.MigrationsDir, version)
	fmt.Printf("  Changes: %d\n", len(changes))

	// Print changes
	for _, change := range changes {
		fmt.Printf("    - %s\n", change.Description)
	}

	return nil
}

// ExtractModelsFromTree extracts models from the syntax tree
func ExtractModelsFromTree(root *tree_sitter.Node, code []byte) []Model {
	var models []Model

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(uint(i))

		switch child.Kind() {
		case "model", "abstract_model":
			model := ParseModel(child, code)
			models = append(models, model)
		}
	}

	return models
}
