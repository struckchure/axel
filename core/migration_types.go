package axel

import "time"

// Migration represents a database migration
type Migration struct {
	Version   string     `json:"version"`    // Sequential: 0001, 0002, etc.
	Name      string     `json:"name"`       // Optional user-provided name
	CreatedAt time.Time  `json:"created_at"` // When migration was created
	AppliedAt *time.Time `json:"applied_at"` // When migration was applied (nil if pending)
	Checksum  string     `json:"checksum"`   // SHA256 of up.sql content
}

// MigrationMetadata stored in metadata.json
type MigrationMetadata struct {
	Version         string    `json:"version"`
	Name            string    `json:"name"`
	CreatedAt       time.Time `json:"created_at"`
	Checksum        string    `json:"checksum"`
	SchemaSnapshot  []Model   `json:"schema_snapshot"`  // Full schema at this migration
	PreviousVersion string    `json:"previous_version"` // Previous migration version
}

// SchemaChange represents a detected change between schemas
type SchemaChange struct {
	Type        ChangeType
	ModelName   string
	FieldName   string // Empty for model-level changes
	OldValue    interface{}
	NewValue    interface{}
	Description string
}

type ChangeType int

const (
	// Model-level changes
	AddModel ChangeType = iota
	DropModel
	RenameModel

	// Field-level changes
	AddField
	DropField
	ModifyField
	RenameField

	// Constraint changes
	AddConstraint
	DropConstraint

	// Index changes
	AddIndex
	DropIndex
)

// MigrationPlan contains the changes and SQL to execute
type MigrationPlan struct {
	Changes []SchemaChange
	UpSQL   string
	DownSQL string
}

// MigrationConfig holds configuration for migration operations
type MigrationConfig struct {
	MigrationsDir string `json:"migrations-dir" yaml:"migrations-dir"` // Default: "./migrations"
	SchemaPath    string `json:"schema-path" yaml:"schema-path"`       // Path to .axel schema file
	DatabaseURL   string `json:"database-url" yaml:"database-url" `    // PostgreSQL connection string
}

// MigrationStatus represents the current state of migrations
type MigrationStatus struct {
	Applied []Migration
	Pending []Migration
}
