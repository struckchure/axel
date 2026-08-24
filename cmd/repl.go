package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/struckchure/axel/core/repl"
)

var replCmd = &cobra.Command{
	Use:     "repl",
	Aliases: []string{"interactive", "i"},
	Short:   "Start an interactive REPL session for testing queries",
	Long: `Start an interactive session for writing and testing AQL queries against a database or compiling to SQL.

Features:
  - Multi-line query editing with auto-continuation
  - Live query execution and compilation
  - Tab autocompletion for keywords, models, fields, and commands
  - Schema inspection (.schema, .models)
  - Parameter management (.param)
  - Multiple output formats: pretty JSON, ASCII table, compact JSON (.format)`,
	// Load config but do not connect migration manager automatically.
	PersistentPreRun: func(cmd *cobra.Command, args []string) { loadConfig() },
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Resolve schema path
		sp, _ := cmd.Flags().GetString("schema-path")
		if sp == "" && config != nil && config.SchemaPath != "" {
			sp = config.SchemaPath
		}
		if sp == "" {
			sp = "axel/schema.asl"
		}

		// Load schema IR if available
		ir, err := loadSchemaIR(sp)
		if err != nil {
			// Don't hard-fail on startup if schema is missing; warn and allow REPL to run
			fmt.Fprintf(os.Stderr, "Warning: failed to load schema from %s (%v)\n", sp, err)
		}

		// Resolve DB URL
		dbURL := ""
		if config != nil && config.DatabaseURL != "" {
			dbURL = config.DatabaseURL
		}

		formatStr, _ := cmd.Flags().GetString("format")
		format := repl.OutputFormat(formatStr)
		if format == "" {
			format = repl.FormatPretty
		}

		relLoadStrategy := ""
		if config != nil {
			relLoadStrategy = config.RelLoadStrategy
		}

		r, err := repl.New(repl.Config{
			DatabaseURL:     dbURL,
			SchemaPath:      sp,
			SchemaIR:        ir,
			Format:          format,
			RelLoadStrategy: relLoadStrategy,
		})
		if err != nil {
			return fmt.Errorf("initializing repl: %w", err)
		}

		return r.Run(ctx)
	},
}

func init() {
	replCmd.Flags().String("schema-path", "", "Schema file, directory or glob (.asl) (default: axel/schema.asl)")
	replCmd.Flags().String("format", "pretty", "Initial output format: pretty, table, compact")
	RootCmd.AddCommand(replCmd)
}
