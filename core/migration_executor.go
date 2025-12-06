package axel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MigrationExecutor handles applying and rolling back migrations
type MigrationExecutor struct {
	manager *MigrationManager
}

// NewMigrationExecutor creates a new migration executor
func NewMigrationExecutor(manager *MigrationManager) *MigrationExecutor {
	return &MigrationExecutor{
		manager: manager,
	}
}

// ApplyMigration applies a single migration
func (e *MigrationExecutor) ApplyMigration(ctx context.Context, migration Migration) error {
	// Read up.sql
	upSQLPath := filepath.Join(e.manager.config.MigrationsDir, migration.Version, "up.sql")
	upSQL, err := os.ReadFile(upSQLPath)
	if err != nil {
		return fmt.Errorf("failed to read up.sql: %w", err)
	}

	// Verify checksum
	actualChecksum := CalculateChecksum(string(upSQL))
	if actualChecksum != migration.Checksum {
		return fmt.Errorf("migration checksum mismatch - migration may have been modified")
	}

	// Start transaction
	tx, err := e.manager.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migration SQL
	startTime := time.Now()
	if _, err := tx.ExecContext(ctx, string(upSQL)); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}
	executionTime := time.Since(startTime).Milliseconds()

	// Record migration in history
	recordQuery := fmt.Sprintf(`
		INSERT INTO %s (version, name, checksum, execution_time_ms)
		VALUES ($1, $2, $3, $4)
	`, MigrationsTable)

	if _, err := tx.ExecContext(ctx, recordQuery, migration.Version, migration.Name, migration.Checksum, executionTime); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	fmt.Printf("Applied migration %s (%s) in %dms\n", migration.Version, migration.Name, executionTime)
	return nil
}

// RollbackMigration rolls back a single migration
func (e *MigrationExecutor) RollbackMigration(ctx context.Context, migration Migration) error {
	// Read down.sql
	downSQLPath := filepath.Join(e.manager.config.MigrationsDir, migration.Version, "down.sql")
	downSQL, err := os.ReadFile(downSQLPath)
	if err != nil {
		return fmt.Errorf("failed to read down.sql: %w", err)
	}

	// Start transaction
	tx, err := e.manager.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute rollback SQL
	startTime := time.Now()
	if _, err := tx.ExecContext(ctx, string(downSQL)); err != nil {
		return fmt.Errorf("failed to execute rollback: %w", err)
	}
	executionTime := time.Since(startTime).Milliseconds()

	// Remove migration from history
	removeQuery := fmt.Sprintf(`DELETE FROM %s WHERE version = $1`, MigrationsTable)
	if _, err := tx.ExecContext(ctx, removeQuery, migration.Version); err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	fmt.Printf("Rolled back migration %s (%s) in %dms\n", migration.Version, migration.Name, executionTime)
	return nil
}

// ApplyPending applies all pending migrations
func (e *MigrationExecutor) ApplyPending(ctx context.Context) error {
	status, err := e.manager.GetStatus(ctx)
	if err != nil {
		return err
	}

	if len(status.Pending) == 0 {
		fmt.Println("No pending migrations")
		return nil
	}

	fmt.Printf("Applying %d pending migration(s)...\n", len(status.Pending))

	for _, migration := range status.Pending {
		if err := e.ApplyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}
	}

	fmt.Println("All migrations applied successfully")
	return nil
}

// Rollback rolls back the last n migrations
func (e *MigrationExecutor) Rollback(ctx context.Context, n int) error {
	status, err := e.manager.GetStatus(ctx)
	if err != nil {
		return err
	}

	if len(status.Applied) == 0 {
		fmt.Println("No applied migrations to rollback")
		return nil
	}

	if n > len(status.Applied) {
		n = len(status.Applied)
	}

	fmt.Printf("Rolling back %d migration(s)...\n", n)

	// Rollback in reverse order
	for i := len(status.Applied) - 1; i >= len(status.Applied)-n; i-- {
		migration := status.Applied[i]
		if err := e.RollbackMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to rollback migration %s: %w", migration.Version, err)
		}
	}

	fmt.Println("Rollback completed successfully")
	return nil
}

// PrintStatus prints the current migration status
func (e *MigrationExecutor) PrintStatus(ctx context.Context) error {
	status, err := e.manager.GetStatus(ctx)
	if err != nil {
		return err
	}

	fmt.Println("\nMigration Status:")
	fmt.Println("================")

	if len(status.Applied) > 0 {
		fmt.Printf("\nApplied (%d):\n", len(status.Applied))
		for _, m := range status.Applied {
			appliedAt := "N/A"
			if m.AppliedAt != nil {
				appliedAt = m.AppliedAt.Format("2006-01-02 15:04:05")
			}
			name := m.Name
			if name == "" {
				name = "(no name)"
			}
			fmt.Printf("  ✓ %s - %s [%s]\n", m.Version, name, appliedAt)
		}
	}

	if len(status.Pending) > 0 {
		fmt.Printf("\nPending (%d):\n", len(status.Pending))
		for _, m := range status.Pending {
			name := m.Name
			if name == "" {
				name = "(no name)"
			}
			fmt.Printf("  ○ %s - %s\n", m.Version, name)
		}
	} else {
		fmt.Println("\nAll migrations are up to date ✓")
	}

	fmt.Println()
	return nil
}
