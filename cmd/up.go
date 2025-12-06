package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	axel "github.com/struckchure/axel/core"
)

// migrateUp applies all pending migrations
func migrateUp() error {
	defer manager.Close()

	// Connect to database
	if err := manager.Connect(); err != nil {
		return err
	}

	// Create executor
	executor := axel.NewMigrationExecutor(manager)

	// Apply pending migrations
	ctx := context.Background()
	return executor.ApplyPending(ctx)
}

var upCmd = &cobra.Command{
	Use: "up",
	Run: func(cmd *cobra.Command, args []string) {
		err := migrateUp()
		if err != nil {
			fmt.Println(err)
		}
	},
}

func init() {
	RootCmd.AddCommand(upCmd)
}
