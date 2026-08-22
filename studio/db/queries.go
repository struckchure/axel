package db

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/struckchure/axel/core/aql"
)

// QueryItem describes a discovered user AQL query file.
type QueryItem struct {
	Name       string       `json:"name"`
	Path       string       `json:"path"`
	Raw        string       `json:"raw"`
	Operation  string       `json:"operation"`
	TargetType string       `json:"target_type"`
	Params     []QueryParam `json:"params"`
}

// QueryParam is an extracted parameter in a query, e.g. $title<str>?.
type QueryParam struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Optional bool   `json:"optional"`
}

var paramRegex = regexp.MustCompile(`\$([a-zA-Z0-9_]+)(?:<([^>]+)>)?(\?)?`)

// FindProjectRoot locates the nearest directory containing axel.yaml, go.mod, or .git.
func FindProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	curr, err := filepath.Abs(dir)
	if err != nil {
		curr = dir
	}
	for {
		if _, err := os.Stat(filepath.Join(curr, "axel.yaml")); err == nil {
			return curr
		}
		if _, err := os.Stat(filepath.Join(curr, "go.mod")); err == nil {
			return curr
		}
		if _, err := os.Stat(filepath.Join(curr, ".git")); err == nil {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr || parent == "" {
			break
		}
		curr = parent
	}
	return dir
}

// DiscoverQueries scans the project root, working directory, and schema directory for *.aql files.
func DiscoverQueries(searchDirs ...string) []QueryItem {
	root := FindProjectRoot()
	allDirs := append([]string{root, "."}, searchDirs...)

	var filePaths []string
	seen := make(map[string]bool)

	for _, dir := range allDirs {
		if dir == "" {
			continue
		}
		// If path is a file, take its directory
		fi, err := os.Stat(dir)
		if err == nil && !fi.IsDir() {
			dir = filepath.Dir(dir)
		}
		if _, err := os.Stat(dir); err != nil {
			continue
		}

		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" || name == "tools" || name == "bin" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".aql") {
				abs, err := filepath.Abs(p)
				if err == nil && !seen[abs] {
					seen[abs] = true
					filePaths = append(filePaths, abs)
				}
			}
			return nil
		})
	}

	var items []QueryItem
	for _, absPath := range filePaths {
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		raw := string(data)

		relPath := absPath
		if rel, err := filepath.Rel(root, absPath); err == nil && !strings.HasPrefix(rel, "..") {
			relPath = rel
		} else if rel, err := filepath.Rel(".", absPath); err == nil && !strings.HasPrefix(rel, "..") {
			relPath = rel
		}

		name := extractQueryName(relPath, raw)
		op, target := detectQueryOperation(raw)
		params := extractQueryParams(raw)

		items = append(items, QueryItem{
			Name:       name,
			Path:       relPath,
			Raw:        raw,
			Operation:  op,
			TargetType: target,
			Params:     params,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func extractQueryName(path, raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@name") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func detectQueryOperation(raw string) (op string, targetType string) {
	stmt, err := aql.ParseString(raw)
	if err == nil && stmt != nil {
		switch {
		case stmt.Select != nil:
			t := ""
			if stmt.Select.Body != nil {
				if stmt.Select.Body.TypeName != "" {
					t = stmt.Select.Body.TypeName
				} else if stmt.Select.Body.AggFunc != nil {
					t = stmt.Select.Body.AggFunc.TypeName
				}
			}
			return "select", t
		case stmt.Insert != nil:
			return "insert", stmt.Insert.TypeName
		case stmt.Update != nil:
			return "update", stmt.Update.TypeName
		case stmt.Delete != nil:
			return "delete", stmt.Delete.TypeName
		}
	}

	// Fallback heuristic if not parsing as single statement
	trimmed := strings.TrimSpace(raw)
	for _, line := range strings.Split(trimmed, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "@") || strings.HasPrefix(l, "//") || strings.HasPrefix(l, "#") || l == "" {
			continue
		}
		fields := strings.Fields(l)
		if len(fields) > 0 {
			lower := strings.ToLower(fields[0])
			if lower == "multi" && len(fields) > 1 {
				lower = strings.ToLower(fields[1])
			}
			switch lower {
			case "select", "insert", "update", "delete":
				var target string
				if len(fields) > 1 && fields[0] != "multi" {
					target = fields[1]
				} else if len(fields) > 2 {
					target = fields[2]
				}
				return lower, strings.TrimRight(target, " {")
			}
		}
	}
	return "query", ""
}

func extractQueryParams(raw string) []QueryParam {
	matches := paramRegex.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var params []QueryParam
	for _, m := range matches {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		paramType := "any"
		if len(m) > 2 && m[2] != "" {
			paramType = m[2]
		}
		optional := len(m) > 3 && m[3] == "?"
		params = append(params, QueryParam{
			Name:     name,
			Type:     paramType,
			Optional: optional,
		})
	}
	return params
}
