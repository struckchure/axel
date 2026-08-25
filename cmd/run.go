package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
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

		// Flags handling
		aqlStr, _ := cmd.Flags().GetString("aql")
		cmdStr, _ := cmd.Flags().GetString("command")
		if aqlStr == "" && cmdStr != "" {
			aqlStr = cmdStr
		}
		aqlFile, _ := cmd.Flags().GetString("file")
		paramFlags, _ := cmd.Flags().GetStringArray("params")
		format, _ := cmd.Flags().GetString("format")
		parallel, _ := cmd.Flags().GetBool("parallel")

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

		// Database connection (shared pool)
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

		// Helper to execute a single AQL source with given positional params
		execOne := func(src string, positionalParams []string) error {
			allParamInputs := append(paramFlags, positionalParams...)
			parsedParams, err := runner.ParseParams(allParamInputs...)
			if err != nil {
				return fmt.Errorf("parsing parameters: %w", err)
			}

			r := runner.New(pool, ir)
			result, err := r.Run(ctx, src, parsedParams)
			if err != nil {
				return fmt.Errorf("running query: %w", err)
			}

			if stmtIsDelete(src) {
				fmt.Printf("{\"rows_affected\": %d}\n", result.RowsAffected)
				return nil
			}

			// Format any UUID fields to string representations
			formattedRows := make([]runner.Row, len(result.Rows))
			for i, row := range result.Rows {
				formattedRows[i] = formatUUIDs(row).(runner.Row)
			}

			var out []byte
			if format == "compact" {
				out, err = json.Marshal(formattedRows)
			} else {
				out, err = json.MarshalIndent(formattedRows, "", "  ")
			}
			if err != nil {
				return fmt.Errorf("formatting output: %w", err)
			}
			fmt.Println(string(out))
			return nil
		}

		// Execution paths
		if aqlStr != "" {
			return execOne(aqlStr, args)
		}

		if aqlFile != "" {
			b, err := os.ReadFile(aqlFile)
			if err != nil {
				return fmt.Errorf("reading --file: %w", err)
			}
			return execOne(string(b), args)
		}

		if len(args) == 0 {
			return fmt.Errorf("a query or .aql file is required (e.g. axel run \"select User { id }\" or axel run query.aql)")
		}

		first := args[0]
		if strings.HasSuffix(first, ".aql") || fileExists(first) {
			files := []string{}
			for _, a := range args {
				if strings.HasSuffix(a, ".aql") || fileExists(a) {
					files = append(files, a)
				} else {
					return fmt.Errorf("unexpected non-.aql argument %s when multiple .aql files are provided", a)
				}
			}

			if parallel {
				var wg sync.WaitGroup
				errCh := make(chan error, len(files))
				for _, f := range files {
					wg.Add(1)
					go func(filePath string) {
						defer wg.Done()
						b, err := os.ReadFile(filePath)
						if err != nil {
							errCh <- fmt.Errorf("reading %s: %w", filePath, err)
							return
						}
						if err := execOne(string(b), nil); err != nil {
							errCh <- err
						}
					}(f)
				}
				wg.Wait()
				close(errCh)
				if len(errCh) > 0 {
					return <-errCh
				}
				return nil
			}

			for _, f := range files {
				b, err := os.ReadFile(f)
				if err != nil {
					return fmt.Errorf("reading %s: %w", f, err)
				}
				if err := execOne(string(b), nil); err != nil {
					return err
				}
			}
			return nil
		}

		return execOne(first, args[1:])
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
	runCmd.Flags().Bool("parallel", false, "Run multiple .aql files concurrently (default sequential)")
	RootCmd.AddCommand(runCmd)
}

func formatUUIDs(v any) any {
	if v == nil {
		return nil
	}
	// 1. [16]byte
	if b, ok := v.([16]byte); ok {
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}
	// 2. *[16]byte
	if b, ok := v.(*[16]byte); ok {
		if b == nil {
			return nil
		}
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}
	// 3. []byte (slice) of length 16
	if b, ok := v.([]byte); ok && len(b) == 16 {
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}
	// 4. pgtype.UUID
	if u, ok := v.(pgtype.UUID); ok {
		if !u.Valid {
			return nil
		}
		b := u.Bytes
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}
	// 5. *pgtype.UUID
	if u, ok := v.(*pgtype.UUID); ok {
		if u == nil || !u.Valid {
			return nil
		}
		b := u.Bytes
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}

	if m, ok := v.(map[string]any); ok {
		for k, val := range m {
			m[k] = formatUUIDs(val)
		}
		return m
	}
	if r, ok := v.(runner.Row); ok {
		for k, val := range r {
			r[k] = formatUUIDs(val)
		}
		return r
	}
	if s, ok := v.([]any); ok {
		for i, val := range s {
			s[i] = formatUUIDs(val)
		}
		return s
	}
	if s, ok := v.([]runner.Row); ok {
		for i, val := range s {
			s[i] = formatUUIDs(val).(runner.Row)
		}
		return s
	}
	return v
}
