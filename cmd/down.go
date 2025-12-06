package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	axel "github.com/struckchure/axel/core"
)

// MigrateDown rolls back migrations
func MigrateDown(steps int) error {
	defer manager.Close()

	// Connect to database
	if err := manager.Connect(); err != nil {
		return err
	}

	// Create executor
	executor := axel.NewMigrationExecutor(manager)

	// Rollback migrations
	ctx := context.Background()
	return executor.Rollback(ctx, steps)
}

var downCmd = &cobra.Command{
	Use:  "down",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		steps, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println(err)
			return
		}

		err = MigrateDown(steps)
		if err != nil {
			fmt.Println(err)
		}
	},
}

func init() {
	RootCmd.AddCommand(downCmd)
}
