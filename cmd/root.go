package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/joho/godotenv"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	axel "github.com/struckchure/axel/core"
	"gopkg.in/yaml.v3"
)

// envRefPattern matches environment-variable references in config values:
// `$env.NAME` or `${env.NAME}`. NAME is a standard shell-style identifier.
var envRefPattern = regexp.MustCompile(`\$\{env\.([A-Za-z_][A-Za-z0-9_]*)\}|\$env\.([A-Za-z_][A-Za-z0-9_]*)`)

// expandEnvRefs replaces `$env.NAME` / `${env.NAME}` references in s with the
// value of the corresponding environment variable. Unset variables expand to
// the empty string.
func expandEnvRefs(s string) string {
	return envRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		m := envRefPattern.FindStringSubmatch(match)
		name := m[1]
		if name == "" {
			name = m[2]
		}
		return os.Getenv(name)
	})
}

// expandConfigEnv expands env references in the string fields of a config.
func expandConfigEnv(c *axel.MigrationConfig) {
	if c == nil {
		return
	}
	c.DatabaseURL = expandEnvRefs(c.DatabaseURL)
	c.SchemaPath = expandEnvRefs(c.SchemaPath)
	c.MigrationsDir = expandEnvRefs(c.MigrationsDir)
	c.ClientDir = expandEnvRefs(c.ClientDir)
	c.PackageName = expandEnvRefs(c.PackageName)
	c.RelLoadStrategy = expandEnvRefs(c.RelLoadStrategy)
	if c.Codegen != nil {
		c.Codegen.Generator = expandEnvRefs(c.Codegen.Generator)
		c.Codegen.Plugin = expandEnvRefs(c.Codegen.Plugin)
		c.Codegen.OutDir = expandEnvRefs(c.Codegen.OutDir)
		for i, q := range c.Codegen.Queries {
			c.Codegen.Queries[i] = expandEnvRefs(q)
		}
		for k, v := range c.Codegen.Options {
			c.Codegen.Options[k] = expandEnvRefs(v)
		}
	}
}

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
// --url always overrides the config file's database URL when set.
//
// Called by both migration commands (full PersistentPreRun) and query commands
// (lightweight override that skips the DB connection).
func loadConfig() {
	// Load .env files first so `$env.NAME` references and env fallbacks below
	// see any variables the user keeps in a .env file.
	loadDotEnv()

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
		expandConfigEnv(config)
		if !lo.IsEmpty(databaseURL) {
			config.DatabaseURL = databaseURL
		}
		if lo.IsEmpty(config.DatabaseURL) {
			config.DatabaseURL = databaseURLFromEnv()
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

	dbURL := databaseURL
	if lo.IsEmpty(dbURL) {
		dbURL = databaseURLFromEnv()
	}

	config = &axel.MigrationConfig{
		DatabaseURL:   dbURL,
		MigrationsDir: md,
		SchemaPath:    sp,
	}
}

// loadDotEnv loads variables from .env files into the process environment
// without overriding variables that are already set. It checks the project
// directory (--dir) first, then the current working directory.
func loadDotEnv() {
	var candidates []string
	if !lo.IsEmpty(projectDir) {
		candidates = append(candidates, projectDir+"/.env")
	}
	candidates = append(candidates, ".env")

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			// godotenv.Load does not override existing environment variables,
			// so real env vars always win over .env entries.
			_ = godotenv.Load(path)
		}
	}
}

// databaseURLFromEnv resolves a database URL from the environment, preferring
// the Axel-specific variable. Returns "" when neither is set.
func databaseURLFromEnv() string {
	if v := os.Getenv("AXEL_DATABASE_URL"); v != "" {
		return v
	}
	return os.Getenv("DATABASE_URL")
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&projectDir, "dir", "d", ".", "Project directory (auto-discovers axel.yaml, schema.asl, or default.asl)")
	RootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Config file path (overrides --dir)")

	RootCmd.PersistentFlags().StringVarP(&databaseURL, "url", "u", "", "Database URL")
	RootCmd.PersistentFlags().StringVar(&migrationsDir, "migrations-dir", "", "Migrations directory")
	RootCmd.PersistentFlags().StringVar(&schemaPath, "schema-path", "", "Schema file path (.asl)")
}
