package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/struckchure/axel/core/asl"
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

		src, err := os.ReadFile(sp)
		if err != nil {
			return fmt.Errorf("reading schema %q: %w", sp, err)
		}

		sf, err := asl.Parse(src)
		if err != nil {
			return fmt.Errorf("parse error: %w", err)
		}

		r := &asl.Resolver{}
		ir, err := r.Resolve(sf)
		if err != nil {
			return fmt.Errorf("resolve error: %w", err)
		}

		errs := asl.Validate(ir)
		if len(errs) > 0 {
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, "  • "+e.Error())
			}
			return fmt.Errorf("schema validation failed:\n%s", strings.Join(msgs, "\n"))
		}

		fmt.Printf("schema %q is valid (%d types)\n", sp, len(ir.ObjectTypes))
		return nil
	},
}

func init() {
	validateCmd.Flags().StringP("schema", "s", "", "Path to .asl schema file (default: axel/schema.asl)")
	RootCmd.AddCommand(validateCmd)
}
