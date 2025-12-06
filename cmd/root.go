package cmd

import (
	"fmt"
	"os"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	axel "github.com/struckchure/axel/core"
	"gopkg.in/yaml.v3"
)

var (
	config  *axel.MigrationConfig
	manager *axel.MigrationManager

	configPath string

	databaseURL   string
	migrationsDir string
	schemaPath    string
)

var RootCmd = &cobra.Command{
	Use:   "axel",
	Short: "Axel is a modern database tool primarily designed for Go, with multi-language support.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if lo.IsEmpty(configPath) {
			config = &axel.MigrationConfig{
				DatabaseURL:   databaseURL,
				MigrationsDir: migrationsDir,
				SchemaPath:    schemaPath,
			}
		} else {
			configData, err := os.ReadFile(configPath)
			if err != nil {
				fmt.Println(err)
				return
			}

			err = yaml.Unmarshal(configData, &config)
			if err != nil {
				fmt.Println(err)
				return
			}
		}

		_manager, err := axel.NewMigrationManager(config)
		if err != nil {
			fmt.Printf("failed to create migration manager: %s", err)
			os.Exit(1)
		}

		manager = _manager
	},
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Config File Path")

	RootCmd.PersistentFlags().StringVarP(&databaseURL, "url", "u", "", "Database URL")
	RootCmd.PersistentFlags().StringVar(&migrationsDir, "migrations-dir", "axel/migrations", "Migrations Dir")
	RootCmd.PersistentFlags().StringVar(&schemaPath, "schema-path", "axel/default.axel", "Schema Path")
}
