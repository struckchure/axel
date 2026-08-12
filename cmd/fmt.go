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
)

var fmtCmd = &cobra.Command{
	Use:   "fmt [path ...]",
	Short: "Format .asl schema and .aql query files",
	Long: `Format Axel source files in canonical style, preserving comments.

Paths may be files or directories; directories are searched recursively for
.asl and .aql files. With no paths, the current directory is formatted.

By default the formatted result is written to stdout. Use --write to update
files in place, or --check to verify formatting (exits non-zero if any file
would change) without writing.`,
	// Formatting needs neither config nor a database connection.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	RunE: func(cmd *cobra.Command, args []string) error {
		write, _ := cmd.Flags().GetBool("write")
		check, _ := cmd.Flags().GetBool("check")
		if write && check {
			return fmt.Errorf("--write and --check are mutually exclusive")
		}

		paths := args
		if len(paths) == 0 {
			paths = []string{"."}
		}
		files, err := collectFmtFiles(paths)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			fmt.Fprintln(os.Stderr, "no .asl or .aql files found")
			return nil
		}

		var unformatted []string
		for _, path := range files {
			src, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading %q: %w", path, err)
			}
			out, err := formatSource(path, src)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			changed := out != string(src)
			switch {
			case check:
				if changed {
					unformatted = append(unformatted, path)
					fmt.Println(path)
				}
			case write:
				if changed {
					if err := os.WriteFile(path, []byte(out), 0644); err != nil {
						return fmt.Errorf("writing %q: %w", path, err)
					}
					fmt.Println(path)
				}
			default:
				fmt.Print(out)
			}
		}
		if check && len(unformatted) > 0 {
			return fmt.Errorf("%d file(s) not formatted", len(unformatted))
		}
		return nil
	},
}

// formatSource dispatches to the right formatter by file extension.
func formatSource(path string, src []byte) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".asl":
		return asl.Format(src)
	case ".aql":
		return aql.Format(src)
	default:
		return "", fmt.Errorf("unsupported file type (want .asl or .aql)")
	}
}

// collectFmtFiles expands the given paths (files or directories) into a
// deduplicated, ordered list of .asl/.aql files.
func collectFmtFiles(paths []string) ([]string, error) {
	seen := make(map[string]bool)
	var files []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", p, err)
		}
		if !info.IsDir() {
			add(p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if ext := strings.ToLower(filepath.Ext(path)); ext == ".asl" || ext == ".aql" {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func init() {
	fmtCmd.Flags().BoolP("write", "w", false, "Write result to (source) file instead of stdout")
	fmtCmd.Flags().Bool("check", false, "List files whose formatting differs; exit non-zero if any")
	RootCmd.AddCommand(fmtCmd)
}
