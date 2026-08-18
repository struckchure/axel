package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate an ASL schema file",
	// Override parent PersistentPreRun — load config but skip DB connection.
	PersistentPreRun: func(cmd *cobra.Command, args []string) { loadConfig() },
	RunE: func(cmd *cobra.Command, args []string) error {
		sp, _ := cmd.Flags().GetString("schema")
		if sp == "" && config != nil && config.SchemaPath != "" {
			sp = config.SchemaPath
		}
		if sp == "" {
			sp = "axel/schema.asl"
		}

		ir, files, err := asl.LoadIR(sp)
		if err != nil {
			return err
		}

		errs := asl.Validate(ir)
		// Inline aql`…` literals are compiled by the migration bridge, which
		// asl.Validate can't reach (import cycle) — check them here so `validate`
		// catches a stale inline query rather than deferring it to `diff`.
		errs = append(errs, validateInlineAQL(ir)...)
		if len(errs) > 0 {
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, "  • "+e.Error())
			}
			return fmt.Errorf("schema validation failed:\n%s", strings.Join(msgs, "\n"))
		}

		if len(files) > 1 {
			fmt.Printf("schema %q is valid (%d files, %d types)\n", sp, len(files), len(ir.ObjectTypes))
		} else {
			fmt.Printf("schema %q is valid (%d types)\n", sp, len(ir.ObjectTypes))
		}
		return nil
	},
}

// validateInlineAQL compiles every inline aql`…` literal in the schema's
// functions, in function-name order, and returns one error per failing query.
func validateInlineAQL(ir *asl.SchemaIR) []error {
	names := make([]string, 0, len(ir.Functions))
	for name := range ir.Functions {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		for _, src := range ir.Functions[name].InlineAQL {
			if _, err := compiler.CompileInline(src, ir); err != nil {
				errs = append(errs, fmt.Errorf("function %q: inline aql %q: %w", name, src, err))
			}
		}
	}
	return errs
}

func init() {
	validateCmd.Flags().StringP("schema", "s", "", "Schema file, directory or glob (.asl) (default: axel/schema.asl)")
	RootCmd.AddCommand(validateCmd)
}
