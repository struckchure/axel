package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		outDir, _ := cmd.Flags().GetString("output-dir")

		// Load schema.
		sp, _ := cmd.Flags().GetString("schema-path")
		if sp == "" && config != nil && config.SchemaPath != "" {
			sp = config.SchemaPath
		}
		if sp == "" {
			sp = "axel/schema.asl"
		}
		ir, err := loadSchemaIR(sp)
		if err != nil {
			return err
		}

		// Batch mode: project dir supplied → glob all *.aql files.
		if projectDir != "" && aqlStr == "" && aqlFile == "" {
			files, err := findAQLFiles(projectDir)
			if err != nil {
				return fmt.Errorf("discovering .aql files in %q: %w", projectDir, err)
			}
			if len(files) == 0 {
				fmt.Fprintf(os.Stderr, "no .aql files found in %q\n", projectDir)
				return nil
			}
			dest := outDir
			if dest == "" {
				dest = projectDir
			}
			if err := os.MkdirAll(dest, 0755); err != nil {
				return fmt.Errorf("creating output directory %q: %w", dest, err)
			}
			for _, f := range files {
				if err := compileFile(f, dest, ir); err != nil {
					return fmt.Errorf("%s: %w", f, err)
				}
			}
			return nil
		}

		// Single-query mode.
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
			return fmt.Errorf("one of --aql, --file, or -d (project dir) is required")
		}

		result, err := compileSrc(src, ir)
		if err != nil {
			return err
		}
		sql := result.Full()

		if outFile != "" {
			if err := os.WriteFile(outFile, []byte(sql+"\n"), 0644); err != nil {
				return fmt.Errorf("writing --out: %w", err)
			}
			fmt.Fprintf(os.Stderr, "written to %s\n", outFile)
		} else if outDir != "" {
			name := "query.sql"
			if aqlFile != "" {
				name = strings.TrimSuffix(filepath.Base(aqlFile), filepath.Ext(aqlFile)) + ".sql"
			}
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("creating output directory %q: %w", outDir, err)
			}
			dest := filepath.Join(outDir, name)
			if err := os.WriteFile(dest, []byte(sql+"\n"), 0644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
			fmt.Fprintf(os.Stderr, "written to %s\n", dest)
		} else {
			fmt.Print(sql)
		}
		return nil
	},
}

func loadSchemaIR(schemaPath string) (*asl.SchemaIR, error) {
	schemaSrc, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("reading schema %q: %w", schemaPath, err)
	}
	sf, err := asl.Parse(schemaSrc)
	if err != nil {
		return nil, fmt.Errorf("parsing schema: %w", err)
	}
	r := &asl.Resolver{}
	ir, err := r.Resolve(sf)
	if err != nil {
		return nil, fmt.Errorf("resolving schema: %w", err)
	}
	return ir, nil
}

func compileSrc(src string, ir *asl.SchemaIR) (*compiler.CompiledSQL, error) {
	stmt, err := aql.ParseString(src)
	if err != nil {
		return nil, fmt.Errorf("parsing AQL: %w", err)
	}
	result, err := compiler.Compile(stmt, ir)
	if err != nil {
		return nil, fmt.Errorf("compiling: %w", err)
	}
	return result, nil
}

// compileFile compiles one .aql file and writes a .sql file to destDir.
func compileFile(aqlPath, destDir string, ir *asl.SchemaIR) error {
	b, err := os.ReadFile(aqlPath)
	if err != nil {
		return fmt.Errorf("reading: %w", err)
	}
	result, err := compileSrc(string(b), ir)
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(filepath.Base(aqlPath), ".aql") + ".sql"
	dest := filepath.Join(destDir, base)
	if err := os.WriteFile(dest, []byte(result.Full()+"\n"), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	fmt.Fprintf(os.Stderr, "compiled %s → %s\n", aqlPath, dest)
	return nil
}

func init() {
	compileCmd.Flags().String("aql", "", "AQL query string")
	compileCmd.Flags().StringP("file", "f", "", "Path to .aql file")
	compileCmd.Flags().StringP("out", "o", "", "Output .sql file (single-file mode, default: stdout)")
	compileCmd.Flags().String("output-dir", "", "Output directory for compiled .sql files")
	compileCmd.Flags().String("schema-path", "", "Path to .asl schema file (default: axel/schema.asl)")
	RootCmd.AddCommand(compileCmd)
}
