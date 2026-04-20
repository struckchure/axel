package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/core/compiler"

	// Register built-in generators.
	_ "github.com/struckchure/axel/generators/golang"
	_ "github.com/struckchure/axel/generators/typescript"
)

var codegenCmd = &cobra.Command{
	Use:   "codegen",
	Short: "Run a codegen plugin against the schema and/or AQL queries",
	Long: `Run a code generator against the resolved ASL schema and optionally a set of AQL queries.

Built-in generators:
  go    Generate Go structs (models.go) and typed query functions (queries.go)

External generators (any language):
  Write a binary that reads a JSON CodegenRequest from stdin and writes a JSON CodegenResponse to stdout.
  See https://github.com/struckchure/axel for the protocol spec.`,
	Args:             cobra.ArbitraryArgs,
	PersistentPreRun: func(cmd *cobra.Command, args []string) { loadConfig() },
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginPath, _ := cmd.Flags().GetString("plugin")
		generatorName, _ := cmd.Flags().GetString("generator")
		outDir, _ := cmd.Flags().GetString("out-dir")
		queryGlobs, _ := cmd.Flags().GetStringArray("query")
		optionPairs, _ := cmd.Flags().GetStringArray("option")

		if pluginPath == "" && generatorName == "" {
			return fmt.Errorf("one of --plugin or --generator is required")
		}
		if pluginPath != "" && generatorName != "" {
			return fmt.Errorf("--plugin and --generator are mutually exclusive")
		}

		// --- Load schema ---
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

		schema := codegen.FromSchemaIR(ir)

		// --- Resolve query file paths ---
		// Sources, in priority order:
		//   1. -q flag values (glob patterns, expanded by axel)
		//   2. Positional args (shell-expanded globs land here)
		//   3. Auto-discovery: all *.aql files under --dir when nothing else given
		var queryPaths []string

		// -q patterns (axel expands these, so quoting works: -q '**/*.aql')
		for _, glob := range queryGlobs {
			matches, err := expandGlob(glob)
			if err != nil {
				return fmt.Errorf("expanding --query %q: %w", glob, err)
			}
			queryPaths = append(queryPaths, matches...)
		}

		// Positional args — shell glob expansion delivers extra files here
		queryPaths = append(queryPaths, args...)

		// Auto-discover when nothing was provided and --dir is set
		if len(queryPaths) == 0 && projectDir != "" {
			discovered, err := findAQLFiles(projectDir)
			if err != nil {
				return fmt.Errorf("discovering .aql files in %q: %w", projectDir, err)
			}
			queryPaths = discovered
		}

		// Deduplicate (shell expansion + -q can produce duplicates)
		queryPaths = dedupe(queryPaths)

		// --- Compile AQL queries ---
		var queries []codegen.QueryDescriptor
		for _, path := range queryPaths {
			src, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading %q: %w", path, err)
			}
			name, aqlSrc := extractQueryName(string(src), path)
			stmt, err := aql.ParseString(aqlSrc)
			if err != nil {
				return fmt.Errorf("parsing AQL %q: %w", path, err)
			}
			compiled, err := compiler.Compile(stmt, ir)
			if err != nil {
				return fmt.Errorf("compiling %q: %w", path, err)
			}
			qd, err := codegen.BuildQueryDescriptor(name, path, stmt, compiled, ir)
			if err != nil {
				return fmt.Errorf("building descriptor for %q: %w", path, err)
			}
			queries = append(queries, qd)
		}

		// --- Build generator ---
		var gen codegen.Generator
		if pluginPath != "" {
			gen = &codegen.SubprocessGenerator{BinaryPath: pluginPath}
		} else {
			gen, err = codegen.Lookup(generatorName)
			if err != nil {
				return err
			}
		}

		// --- Parse options ---
		options := map[string]string{}
		for _, pair := range optionPairs {
			k, v, found := strings.Cut(pair, "=")
			if !found {
				return fmt.Errorf("invalid --option %q: expected key=value", pair)
			}
			options[k] = v
		}

		ctx := &codegen.Context{
			OutDir:  outDir,
			Options: options,
		}

		// --- Run ---
		if err := codegen.Walk(schema, queries, gen, ctx); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "codegen complete → %s\n", outDir)
		return nil
	},
}

func init() {
	codegenCmd.Flags().StringP("plugin", "p", "", "Path to external generator binary")
	codegenCmd.Flags().StringP("generator", "g", "", "Built-in generator name (e.g. go)")
	codegenCmd.Flags().StringP("out-dir", "o", ".", "Output directory for generated files")
	codegenCmd.Flags().String("schema-path", "", "Path to .asl schema (default: from config or axel/schema.asl)")
	codegenCmd.Flags().StringArrayP("query", "q", nil, "AQL query file or glob (repeatable)")
	codegenCmd.Flags().StringArray("option", nil, "key=value option forwarded to the generator (repeatable)")
	RootCmd.AddCommand(codegenCmd)
}

// expandGlob expands a glob pattern. Supports ** for recursive matching.
// Plain paths with no wildcards are returned as-is.
func expandGlob(pattern string) ([]string, error) {
	if strings.Contains(pattern, "**") {
		// Split on ** and walk the directory portion.
		parts := strings.SplitN(pattern, "**", 2)
		dir := filepath.Clean(parts[0])
		suffix := strings.TrimPrefix(parts[1], string(filepath.Separator))
		return findAQLFilesWithSuffix(dir, suffix)
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return []string{pattern}, nil
	}
	return matches, nil
}

// findAQLFiles walks dir recursively and returns all *.aql files.
func findAQLFiles(dir string) ([]string, error) {
	return findAQLFilesWithSuffix(dir, "*.aql")
}

// findAQLFilesWithSuffix walks dir recursively and returns files matching suffix glob.
// suffix is matched against the filename only (e.g. "*.aql").
func findAQLFilesWithSuffix(dir, suffix string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if suffix == "" || suffix == "*.aql" {
			if strings.HasSuffix(path, ".aql") {
				files = append(files, path)
			}
			return nil
		}
		matched, _ := filepath.Match(suffix, filepath.Base(path))
		if matched {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// dedupe returns paths with duplicates removed, preserving order.
func dedupe(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := paths[:0]
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// extractQueryName checks for a "# @name Foo" annotation on the first line.
// Returns (name, aqlSource). If no annotation, name is derived from path.
func extractQueryName(src, path string) (string, string) {
	line, rest, _ := strings.Cut(src, "\n")
	line = strings.TrimSpace(line)
	if after, ok := strings.CutPrefix(line, "# @name"); ok {
		name := strings.TrimSpace(after)
		if name != "" {
			return name, rest
		}
	}
	return "", src
}
