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

	projectDir string
	configPath string

	databaseURL   string
	migrationsDir string
	schemaPath    string
)

var RootCmd = &cobra.Command{
	Use:           "axel",
	Short:         "Axel — schema (ASL) and query (AQL) languages that compile to SQL.",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		loadConfig()

		_manager, err := axel.NewMigrationManager(config)
		if err != nil {
			fmt.Printf("failed to create migration manager: %s", err)
			os.Exit(1)
		}

		manager = _manager
	},
}

// loadConfig populates the global config from flags, resolving in this order:
//  1. --config explicit path
//  2. --dir/axel.yaml (auto-discovered)
//  3. Individual --url / --schema-path / --migrations-dir flags
//
// Called by both migration commands (full PersistentPreRun) and query commands
// (lightweight override that skips the DB connection).
func loadConfig() {
	// Resolve an explicit --config path.
	resolved := configPath

	// If --dir was given and no explicit --config, look for axel.yaml there.
	if lo.IsEmpty(resolved) && !lo.IsEmpty(projectDir) {
		candidate := projectDir + "/axel.yaml"
		if _, err := os.Stat(candidate); err == nil {
			resolved = candidate
		}
	}

	if !lo.IsEmpty(resolved) {
		configData, err := os.ReadFile(resolved)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err = yaml.Unmarshal(configData, &config); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return
	}

	// Fall back to individual flags. If --dir was given, derive defaults from it.
	sp := schemaPath
	md := migrationsDir
	if !lo.IsEmpty(projectDir) {
		if lo.IsEmpty(sp) || sp == "axel/schema.asl" {
			// Prefer schema.asl, fall back to default.asl.
			if _, err := os.Stat(projectDir + "/schema.asl"); err == nil {
				sp = projectDir + "/schema.asl"
			} else if _, err := os.Stat(projectDir + "/default.asl"); err == nil {
				sp = projectDir + "/default.asl"
			}
		}
		if lo.IsEmpty(md) || md == "axel/migrations" {
			md = projectDir + "/migrations"
		}
	}

	config = &axel.MigrationConfig{
		DatabaseURL:   databaseURL,
		MigrationsDir: md,
		SchemaPath:    sp,
	}
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&projectDir, "dir", "d", "", "Project directory (auto-discovers axel.yaml, schema.asl, or default.asl)")
	RootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Config file path (overrides --dir)")

	RootCmd.PersistentFlags().StringVarP(&databaseURL, "url", "u", "", "Database URL")
	RootCmd.PersistentFlags().StringVar(&migrationsDir, "migrations-dir", "", "Migrations directory")
	RootCmd.PersistentFlags().StringVar(&schemaPath, "schema-path", "", "Schema file path (.asl)")
}
