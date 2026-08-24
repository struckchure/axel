package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/struckchure/axel/core/runner"
)

var runCmd = &cobra.Command{
	Use:     "run [query|file] [params...]",
	Aliases: []string{"r"},
	Short:   "Execute an AQL query directly against the database",
	Long: `Execute an AQL query against the database with parameters.

Parameters can be passed in several formats:
  - Flag: --params '{"skip": 1, "limit": 20}'
  - Relaxed JSON: --params '{skip: 1, limit: 20}'
  - Prefixed: --params 'params={skip: 1, limit: 20}'
  - Key-value: -p skip=1 -p limit=20
  - Positional: axel run "select User { id }" skip=1 limit=20
  - Positional JSON: axel run user.aql 'params={skip: 1, limit: 20}'`,
	// Load config but do not connect migration manager automatically.
	PersistentPreRun: func(cmd *cobra.Command, args []string) { loadConfig() },
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		aqlStr, _ := cmd.Flags().GetString("aql")
		cmdStr, _ := cmd.Flags().GetString("command")
		if aqlStr == "" && cmdStr != "" {
			aqlStr = cmdStr
		}
		aqlFile, _ := cmd.Flags().GetString("file")
		paramFlags, _ := cmd.Flags().GetStringArray("params")
		format, _ := cmd.Flags().GetString("format")

		// Load schema
		sp, _ := cmd.Flags().GetString("schema-path")
		if sp == "" && config != nil && config.SchemaPath != "" {
			sp = config.SchemaPath
		}
		if sp == "" {
			sp = "axel/schema.asl"
		}
		ir, err := loadSchemaIR(sp)
		if err != nil {
			return fmt.Errorf("loading schema: %w", err)
		}

		// Resolve query source and positional param arguments
		var src string
		var positionalParams []string

		switch {
		case aqlStr != "":
			src = aqlStr
			positionalParams = args
		case aqlFile != "":
			b, err := os.ReadFile(aqlFile)
			if err != nil {
				return fmt.Errorf("reading --file: %w", err)
			}
			src = string(b)
			positionalParams = args
		case len(args) > 0:
			first := args[0]
			if strings.HasSuffix(first, ".aql") || fileExists(first) {
				b, err := os.ReadFile(first)
				if err != nil {
					return fmt.Errorf("reading aql file %s: %w", first, err)
				}
				src = string(b)
			} else {
				src = first
			}
			positionalParams = args[1:]
		default:
			return fmt.Errorf("a query or .aql file is required (e.g. axel run \"select User { id }\" or axel run query.aql)")
		}

		// Collect and parse all parameter inputs
		var allParamInputs []string
		allParamInputs = append(allParamInputs, paramFlags...)
		allParamInputs = append(allParamInputs, positionalParams...)

		params, err := runner.ParseParams(allParamInputs...)
		if err != nil {
			return fmt.Errorf("parsing parameters: %w", err)
		}

		// Connect to DB
		dbURL := ""
		if config != nil && config.DatabaseURL != "" {
			dbURL = config.DatabaseURL
		}
		if dbURL == "" {
			return fmt.Errorf("no database URL configured; specify --url or set DATABASE_URL")
		}

		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			return fmt.Errorf("connecting to database: %w", err)
		}
		defer pool.Close()

		r := runner.New(pool, ir)
		result, err := r.Run(ctx, src, params)
		if err != nil {
			return fmt.Errorf("running query: %w", err)
		}

		// Output result
		if stmtIsDelete(src) {
			fmt.Printf("{\"rows_affected\": %d}\n", result.RowsAffected)
			return nil
		}

		var out []byte
		if format == "compact" {
			out, err = json.Marshal(result.Rows)
		} else {
			out, err = json.MarshalIndent(result.Rows, "", "  ")
		}
		if err != nil {
			return fmt.Errorf("formatting output: %w", err)
		}

		fmt.Println(string(out))
		return nil
	},
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func stmtIsDelete(src string) bool {
	trimmed := strings.TrimSpace(src)
	return strings.HasPrefix(trimmed, "delete ") || strings.HasPrefix(trimmed, "delete\n")
}

func init() {
	runCmd.Flags().StringP("command", "c", "", "AQL query string to execute")
	runCmd.Flags().StringP("aql", "q", "", "AQL query string to execute (alias for -c)")
	runCmd.Flags().StringP("file", "f", "", "Path to .aql file")
	runCmd.Flags().StringArrayP("params", "p", nil, "Query parameters in JSON or key=value format (e.g. '{\"skip\": 1, \"limit\": 20}' or 'params={skip: 1, limit: 20}')")
	runCmd.Flags().String("format", "pretty", "Output format: pretty, compact")
	runCmd.Flags().String("schema-path", "", "Schema file, directory or glob (.asl) (default: axel/schema.asl)")
	RootCmd.AddCommand(runCmd)
}
