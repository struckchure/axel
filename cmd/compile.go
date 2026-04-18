package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

var compileCmd = &cobra.Command{
	Use:   "compile",
	Short: "Compile AQL query to parameterized SQL",
	// Override parent PersistentPreRun — load config but skip DB connection.
	PersistentPreRun: func(cmd *cobra.Command, args []string) { loadConfig() },
	RunE: func(cmd *cobra.Command, args []string) error {
		aqlStr, _ := cmd.Flags().GetString("aql")
		aqlFile, _ := cmd.Flags().GetString("file")
		outFile, _ := cmd.Flags().GetString("out")

		// Load AQL source.
		var src string
		switch {
		case aqlStr != "":
			src = aqlStr
		case aqlFile != "":
			b, err := os.ReadFile(aqlFile)
			if err != nil {
				return fmt.Errorf("reading --file: %w", err)
			}
			src = string(b)
		default:
			return fmt.Errorf("one of --aql or --file is required")
		}

		// Load schema: command flag > config file > default.
		sp, _ := cmd.Flags().GetString("schema-path")
		if sp == "" && config != nil && config.SchemaPath != "" {
			sp = config.SchemaPath
		}
		if sp == "" {
			sp = "axel/schema.asl"
		}
		schemaSrc, err := os.ReadFile(sp)
		if err != nil {
			return fmt.Errorf("reading schema %q: %w", sp, err)
		}
		sf, err := asl.Parse(schemaSrc)
		if err != nil {
			return fmt.Errorf("parsing schema: %w", err)
		}
		r := &asl.Resolver{}
		ir, err := r.Resolve(sf)
		if err != nil {
			return fmt.Errorf("resolving schema: %w", err)
		}

		// Parse AQL.
		stmt, err := aql.ParseString(src)
		if err != nil {
			return fmt.Errorf("parsing AQL: %w", err)
		}

		// Compile.
		result, err := compiler.Compile(stmt, ir)
		if err != nil {
			return fmt.Errorf("compiling: %w", err)
		}

		sql := result.Full()

		// Output.
		if outFile != "" {
			if err := os.WriteFile(outFile, []byte(sql+"\n"), 0644); err != nil {
				return fmt.Errorf("writing --out: %w", err)
			}
			fmt.Fprintf(os.Stderr, "written to %s\n", outFile)
		} else {
			fmt.Print(sql)
		}
		return nil
	},
}

func init() {
	compileCmd.Flags().String("aql", "", "AQL query string")
	compileCmd.Flags().StringP("file", "f", "", "Path to .aql file")
	compileCmd.Flags().StringP("out", "o", "", "Output .sql file (default: stdout)")
	compileCmd.Flags().String("schema-path", "", "Path to .asl schema file (default: axel/schema.asl)")
	RootCmd.AddCommand(compileCmd)
}
