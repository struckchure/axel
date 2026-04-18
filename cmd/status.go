package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	axel "github.com/struckchure/axel/core"
)

// migrateStatus shows the current migration status
func migrateStatus() error {
	defer manager.Close()

	// Connect to database
	if err := manager.Connect(); err != nil {
		return err
	}

	// Create executor
	executor := axel.NewMigrationExecutor(manager)

	// Print status
	ctx := context.Background()
	return executor.PrintStatus(ctx)
}

var statusCmd = &cobra.Command{
	Use: "status",
	Run: func(cmd *cobra.Command, args []string) {
		err := migrateStatus()
		if err != nil {
			fmt.Println(err)
		}
	},
}

func init() {
	RootCmd.AddCommand(statusCmd)
}
