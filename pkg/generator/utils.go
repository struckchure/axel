package generator

import "strings"

// toPascalCase converts a string to PascalCase
func toPascalCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, "")
}

// toCamelCase converts a string to camelCase
func toCamelCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, word := range words {
		if len(word) > 0 {
			if i == 0 {
				words[i] = strings.ToLower(word)
			} else {
				words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
			}
		}
	}
	return strings.Join(words, "")
}

// toSnakeCase converts a string to snake_case (or preserves it if already snake_case)
func toSnakeCase(s string) string {
	// If already contains underscores, just lowercase it
	if strings.Contains(s, "_") {
		return strings.ToLower(s)
	}
	// Otherwise, return as-is (assuming it's already in correct format)
	return strings.ToLower(s)
}

// toKebabCase converts a string to kebab-case
func toKebabCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == ' '
	})
	return strings.ToLower(strings.Join(words, "-"))
}
