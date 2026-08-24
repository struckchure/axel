package repl

import (
	"strings"
)

// IsCompleteStatement reports whether input forms a complete statement that can
// be executed immediately, or if the REPL should prompt for additional lines.
func IsCompleteStatement(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return true
	}

	// Dot / backslash meta-commands are always single-line.
	if strings.HasPrefix(trimmed, ".") || strings.HasPrefix(trimmed, "\\") {
		return true
	}

	// Trailing semicolon forces statement completion.
	if strings.HasSuffix(trimmed, ";") {
		return true
	}

	var (
		braceDepth   int
		parenDepth   int
		bracketDepth int
		inSingle     bool
		inDouble     bool
		inBacktick   bool
		escaped      bool
	)

	runes := []rune(input)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]

		if escaped {
			escaped = false
			continue
		}

		if r == '\\' && (inSingle || inDouble || inBacktick) {
			escaped = true
			continue
		}

		if inSingle {
			if r == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if r == '"' {
				inDouble = false
			}
			continue
		}
		if inBacktick {
			if r == '`' {
				inBacktick = false
			}
			continue
		}

		// Check for line comments (# or -- or //)
		if r == '#' {
			// skip till newline
			for i < n && runes[i] != '\n' {
				i++
			}
			continue
		}
		if r == '-' && i+1 < n && runes[i+1] == '-' {
			for i < n && runes[i] != '\n' {
				i++
			}
			continue
		}
		if r == '/' && i+1 < n && runes[i+1] == '/' {
			for i < n && runes[i] != '\n' {
				i++
			}
			continue
		}

		switch r {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '`':
			inBacktick = true
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
	}

	if inSingle || inDouble || inBacktick {
		return false
	}

	if braceDepth > 0 || parenDepth > 0 || bracketDepth > 0 {
		return false
	}

	// Check if the query ends in a trailing operator or keyword that expects continuation
	lastWord := trailingWord(trimmed)
	switch strings.ToLower(lastWord) {
	case "with", "filter", "order", "by", "group", "having", "set", "and", "or", "where", ",":
		return false
	}

	return true
}

func trailingWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
