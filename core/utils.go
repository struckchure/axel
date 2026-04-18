package axel

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samber/lo"
)

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func formatIdentifier(s string) string {
	return fmt.Sprintf(`"%s"`, lo.SnakeCase(s))
}

func TransformTemplate(content string) ([]string, error) {
	lines := strings.Split(content, "\n")
	result := []string{}

	i := 0
	for i < len(lines) {
		line := lines[i]

		// Check for @replace directive
		if match := regexp.MustCompile(`// @replace (.+)`).FindStringSubmatch(line); match != nil {
			i++ // Skip the @replace line
			if i < len(lines) {
				i++ // Skip the next line (the one being replaced)
			}
			result = append(result, match[1])
			continue
		}

		// Check for @block directive
		if match := regexp.MustCompile(`// @block (.+)`).FindStringSubmatch(line); match != nil {
			expression := match[1]
			i++ // Skip the @block line
			result = append(result, expression)
			continue
		}

		// Check for @start directive
		if match := regexp.MustCompile(`// @start (.+)`).FindStringSubmatch(line); match != nil {
			startTemplate := match[1]
			i++ // Skip the @start line

			// Read the next line - check if it's a @block directive
			var blockTemplate string
			if i < len(lines) {
				nextLine := lines[i]
				if match := regexp.MustCompile(`// @block (.+)`).FindStringSubmatch(nextLine); match != nil {
					blockTemplate = match[1]
				} else {
					blockTemplate = nextLine
				}
				i++ // Skip the template line
			}

			// Find the @end directive and skip all lines until then
			var endTemplate string
			for i < len(lines) {
				if endMatch := regexp.MustCompile(`// @end (.+)`).FindStringSubmatch(lines[i]); endMatch != nil {
					endTemplate = endMatch[1]
					i++ // Skip the @end line
					break
				}
				i++
			}

			// Append the template and end template
			result = append(result, startTemplate)
			result = append(result, blockTemplate)
			result = append(result, endTemplate)

			continue
		}

		result = append(result, line)
		i++
	}

	return result, nil
}

func ExecuteTemplate(name, tmpl string, data any, funcMaps ...template.FuncMap) (*string, error) {
	var buf bytes.Buffer

	t := template.New(name)

	// Apply function maps if provided
	if len(funcMaps) > 0 {
		funcMap := funcMaps[0]
		t = t.Funcs(funcMap)
	}

	// Parse the template
	t, err := t.Parse(tmpl)
	if err != nil {
		return nil, err
	}

	err = t.Execute(&buf, data)
	if err != nil {
		return nil, err
	}

	return lo.ToPtr(buf.String()), nil
}

func WriteFile(filename string, data []byte, perm os.FileMode) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write the file
	if err := os.WriteFile(filename, data, perm); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
