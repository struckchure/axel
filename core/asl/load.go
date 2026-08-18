package asl

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Ext is the file extension of an Axel Schema Language file.
const Ext = ".asl"

// ExpandPaths resolves a schema path spec into a sorted, deduplicated list of
// .asl files. spec may be:
//
//	schema.asl          a single file
//	schema/             a directory, walked recursively
//	schema/*.asl        a glob
//	schema/**/*.asl     a recursive glob
//
// A spec that matches no .asl file is an error: silently resolving an empty
// schema would make `diff` generate a migration that drops everything.
func ExpandPaths(spec string) ([]string, error) {
	if spec == "" {
		return nil, fmt.Errorf("no schema path given")
	}

	var (
		files []string
		err   error
	)
	switch {
	case !hasMeta(spec):
		info, statErr := os.Stat(spec)
		if statErr != nil {
			return nil, statErr
		}
		if info.IsDir() {
			files, err = walkASL(spec, "")
		} else {
			files = []string{spec}
		}

	case strings.Contains(spec, "**"):
		parts := strings.SplitN(spec, "**", 2)
		dir := filepath.Clean(parts[0])
		if dir == "" {
			dir = "."
		}
		suffix := strings.TrimPrefix(parts[1], string(filepath.Separator))
		files, err = walkASL(dir, suffix)

	default:
		var matches []string
		matches, err = filepath.Glob(spec)
		for _, m := range matches {
			if strings.EqualFold(filepath.Ext(m), Ext) {
				files = append(files, m)
			}
		}
	}
	if err != nil {
		return nil, err
	}

	files = sortDedupe(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no %s files match %q", Ext, spec)
	}
	return files, nil
}

// Load expands spec, then reads, parses and merges every matching file into a
// single AST. It also returns the files it read, in the order they were merged.
func Load(spec string) (*SourceFile, []string, error) {
	paths, err := ExpandPaths(spec)
	if err != nil {
		return nil, nil, err
	}

	parsed := make([]*SourceFile, 0, len(paths))
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, fmt.Errorf("reading schema %q: %w", p, err)
		}
		sf, err := ParseNamed(p, src)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing schema %q: %w", p, err)
		}
		parsed = append(parsed, sf)
	}
	return Merge(parsed...), paths, nil
}

// LoadIR is Load followed by Resolve: it turns a schema path spec into the
// resolved IR every downstream stage (migrations, codegen, AQL) consumes.
func LoadIR(spec string) (*SchemaIR, []string, error) {
	sf, paths, err := Load(spec)
	if err != nil {
		return nil, nil, err
	}
	ir, err := (&Resolver{}).Resolve(sf)
	if err != nil {
		return nil, paths, fmt.Errorf("resolving schema: %w", err)
	}
	return ir, paths, nil
}

// Merge concatenates the definitions of several source files, in order, into
// one AST. Files share a single flat namespace — there is no per-file scoping,
// and the resolver rejects a name declared in two of them.
func Merge(files ...*SourceFile) *SourceFile {
	merged := &SourceFile{}
	for _, f := range files {
		if f == nil {
			continue
		}
		merged.Definitions = append(merged.Definitions, f.Definitions...)
	}
	return merged
}

// hasMeta reports whether spec contains glob metacharacters.
func hasMeta(spec string) bool {
	return strings.ContainsAny(spec, "*?[")
}

// walkASL walks dir recursively and returns every .asl file. When suffix is
// non-empty it is matched (as a glob) against each file's path relative to dir,
// and against its base name, so both `schema/**/*.asl` and `schema/**/user.asl`
// behave as expected.
func walkASL(dir, suffix string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), Ext) {
			return nil
		}
		if suffix != "" && !matchSuffix(dir, path, suffix) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func matchSuffix(dir, path, suffix string) bool {
	if ok, _ := filepath.Match(suffix, filepath.Base(path)); ok {
		return true
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	ok, _ := filepath.Match(suffix, rel)
	return ok
}

// skipDir reports whether a directory should be excluded from a recursive walk.
func skipDir(name string) bool {
	return name == "node_modules" || strings.HasPrefix(name, ".")
}

func sortDedupe(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		clean := filepath.Clean(p)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}
