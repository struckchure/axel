package main

import (
	"os"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/struckchure/axel/studio"
)

var (
	studioAddr       string
	studioQueryGlobs []string
)

var studioCmd = &cobra.Command{
	Use:   "studio",
	Short: "Launch Axel Studio — a browser database viewer & editor (AQL/SQL).",
	Long: `Launch Axel Studio, a Neon/Prisma-Studio-style viewer for your database.

Browse tables as ASL types, inspect their structure, and edit rows inline —
inserts, updates and deletes are applied as AQL. An AQL console (and a raw SQL
console) run queries against the connected database.

Connection and schema resolve from --url / --schema-path, then axel.yaml, then
the AXEL_DATABASE_URL / DATABASE_URL / AXEL_SCHEMA_PATH environment variables.`,
	// Override the parent PersistentPreRun: load config but skip the migration
	// manager's DB connection — the studio manages its own pool.
	PersistentPreRun: func(cmd *cobra.Command, args []string) { loadConfig() },
	RunE: func(cmd *cobra.Command, args []string) error {
		url := firstNonEmpty(databaseURL, configDatabaseURL(), envOr("AXEL_DATABASE_URL", os.Getenv("DATABASE_URL")))
		sp := firstNonEmpty(schemaPath, configSchemaPath(), envOr("AXEL_SCHEMA_PATH", defaultSchemaPath()))

		var qPaths []string
		qPaths = append(qPaths, studioQueryGlobs...)
		if config != nil && config.Codegen != nil {
			qPaths = append(qPaths, config.Codegen.Queries...)
		}

		return studio.Run(studio.Options{
			Addr:        studioAddr,
			DatabaseURL: url,
			SchemaPath:  sp,
			QueryPaths:  qPaths,
		})
	},
}

func configDatabaseURL() string {
	if config != nil {
		return config.DatabaseURL
	}
	return ""
}

func configSchemaPath() string {
	if config != nil {
		return config.SchemaPath
	}
	return ""
}

// defaultSchemaPath picks a dev-time schema if one exists at a known location.
func defaultSchemaPath() string {
	for _, p := range []string{"default.asl", "../examples/basic/default.asl", "examples/basic/default.asl"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if !lo.IsEmpty(v) {
			return v
		}
	}
	return ""
}

func init() {
	studioCmd.Flags().StringVar(&studioAddr, "addr", studio.DefaultAddr, "listen address")
	studioCmd.Flags().StringArrayVarP(&studioQueryGlobs, "query", "q", nil, "AQL query file or glob (repeatable)")
	RootCmd.AddCommand(studioCmd)
}
