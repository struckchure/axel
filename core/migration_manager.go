package axel

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "github.com/lib/pq"
)

const MigrationsTable = "_axel_migrations"

// MigrationManager handles all migration operations
type MigrationManager struct {
	config *MigrationConfig
	db     *sql.DB
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(config *MigrationConfig) (*MigrationManager, error) {
	if config.MigrationsDir != "" {
		if err := os.MkdirAll(config.MigrationsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create migrations directory: %w", err)
		}
	}

	return &MigrationManager{
		config: config,
	}, nil
}

// Connect establishes database connection
func (m *MigrationManager) Connect() error {
	if m.db != nil {
		return nil // Already connected
	}

	db, err := sql.Open("postgres", m.config.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	m.db = db
	return nil
}

// Close closes the database connection
func (m *MigrationManager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// EnsureMigrationsTable creates the migrations tracking table if it doesn't exist
func (m *MigrationManager) EnsureMigrationsTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			checksum VARCHAR(64) NOT NULL,
			execution_time_ms INTEGER NOT NULL
		);
	`, MigrationsTable)

	_, err := m.db.ExecContext(ctx, query)
	return err
}

// GetAppliedMigrations returns all migrations that have been applied
func (m *MigrationManager) GetAppliedMigrations(ctx context.Context) ([]Migration, error) {
	if err := m.EnsureMigrationsTable(ctx); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT version, name, applied_at, checksum
		FROM %s
		ORDER BY version ASC
	`, MigrationsTable)

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []Migration
	for rows.Next() {
		var m Migration
		if err := rows.Scan(&m.Version, &m.Name, &m.AppliedAt, &m.Checksum); err != nil {
			return nil, err
		}
		migrations = append(migrations, m)
	}

	return migrations, rows.Err()
}

// GetAvailableMigrations scans the migrations directory for all migrations
func (m *MigrationManager) GetAvailableMigrations() ([]Migration, error) {
	entries, err := os.ReadDir(m.config.MigrationsDir)
	if err != nil {
		return nil, err
	}

	var migrations []Migration
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Read metadata
		metadataPath := filepath.Join(m.config.MigrationsDir, entry.Name(), "metadata.json")
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			continue // Skip invalid migrations
		}

		var metadata MigrationMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			continue
		}

		migrations = append(migrations, Migration{
			Version:   metadata.Version,
			Name:      metadata.Name,
			CreatedAt: metadata.CreatedAt,
			Checksum:  metadata.Checksum,
		})
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// GetStatus returns the current migration status
func (m *MigrationManager) GetStatus(ctx context.Context) (*MigrationStatus, error) {
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	available, err := m.GetAvailableMigrations()
	if err != nil {
		return nil, err
	}

	// Create map of applied versions
	appliedMap := make(map[string]bool)
	for _, m := range applied {
		appliedMap[m.Version] = true
	}

	// Determine pending migrations
	var pending []Migration
	for _, m := range available {
		if !appliedMap[m.Version] {
			pending = append(pending, m)
		}
	}

	return &MigrationStatus{
		Applied: applied,
		Pending: pending,
	}, nil
}

// RecordMigration records a migration as applied
func (m *MigrationManager) RecordMigration(ctx context.Context, version, name, checksum string, executionTime int64) error {
	query := fmt.Sprintf(`
		INSERT INTO %s (version, name, checksum, execution_time_ms)
		VALUES ($1, $2, $3, $4)
	`, MigrationsTable)

	_, err := m.db.ExecContext(ctx, query, version, name, checksum, executionTime)
	return err
}

// RemoveMigration removes a migration record (used during rollback)
func (m *MigrationManager) RemoveMigration(ctx context.Context, version string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE version = $1`, MigrationsTable)
	_, err := m.db.ExecContext(ctx, query, version)
	return err
}

// GetNextVersion returns the next migration version number
func (m *MigrationManager) GetNextVersion() (string, error) {
	migrations, err := m.GetAvailableMigrations()
	if err != nil {
		return "", err
	}

	if len(migrations) == 0 {
		return "0001", nil
	}

	// Get last version and increment
	lastVersion := migrations[len(migrations)-1].Version
	var num int
	fmt.Sscanf(lastVersion, "%d", &num)
	return fmt.Sprintf("%04d", num+1), nil
}

// GetLastSchema returns the schema snapshot from the last migration
func (m *MigrationManager) GetLastSchema() ([]Model, error) {
	migrations, err := m.GetAvailableMigrations()
	if err != nil {
		return nil, err
	}

	if len(migrations) == 0 {
		return []Model{}, nil // No previous schema
	}

	// Read metadata from last migration
	lastMigration := migrations[len(migrations)-1]
	metadataPath := filepath.Join(m.config.MigrationsDir, lastMigration.Version, "metadata.json")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata MigrationMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return metadata.SchemaSnapshot, nil
}

// CalculateChecksum computes SHA256 hash of content
func CalculateChecksum(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// CreateMigrationDir creates a new migration directory with metadata
func (m *MigrationManager) CreateMigrationDir(version, name string, schema []Model, upSQL, downSQL string) error {
	migrationDir := filepath.Join(m.config.MigrationsDir, version)
	if err := os.MkdirAll(migrationDir, 0755); err != nil {
		return err
	}

	// Write up.sql
	upPath := filepath.Join(migrationDir, "up.sql")
	if err := os.WriteFile(upPath, []byte(upSQL), 0644); err != nil {
		return err
	}

	// Write down.sql
	downPath := filepath.Join(migrationDir, "down.sql")
	if err := os.WriteFile(downPath, []byte(downSQL), 0644); err != nil {
		return err
	}

	// Get previous version
	previousVersion := ""
	if version != "0001" {
		var num int
		fmt.Sscanf(version, "%d", &num)
		previousVersion = fmt.Sprintf("%04d", num-1)
	}

	// Create metadata
	metadata := MigrationMetadata{
		Version:         version,
		Name:            name,
		CreatedAt:       time.Now(),
		Checksum:        CalculateChecksum(upSQL),
		SchemaSnapshot:  schema,
		PreviousVersion: previousVersion,
	}

	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	metadataPath := filepath.Join(migrationDir, "metadata.json")
	return os.WriteFile(metadataPath, metadataBytes, 0644)
}
