package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var unquotedKeyRegex = regexp.MustCompile(`([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)

// ParseParams parses parameter inputs provided via CLI flags or arguments.
// Supports:
//   - JSON string: `{"skip": 1, "limit": 20}`
//   - Relaxed JSON: `{skip: 1, limit: 20}`
//   - Prefixed JSON: `params={skip: 1, limit: 20}` or `params={"skip": 1, "limit": 20}`
//   - Key-value pairs: `skip=1`, `limit=20`, `name='Alice'`
//   - File paths starting with `@`: `@params.json`
func ParseParams(inputs ...string) (map[string]any, error) {
	result := make(map[string]any)
	for _, raw := range inputs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		// Handle @filename
		if strings.HasPrefix(raw, "@") {
			content, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
			if err != nil {
				return nil, fmt.Errorf("reading params file: %w", err)
			}
			raw = strings.TrimSpace(string(content))
		}

		// Handle params=... or params:... prefix
		if strings.HasPrefix(raw, "params=") {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "params="))
		} else if strings.HasPrefix(raw, "params:") {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "params:"))
		}

		// If it looks like a JSON object {...}
		if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
			var parsed map[string]any
			// Try strict unmarshal first
			if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
				for k, v := range parsed {
					result[k] = v
				}
				continue
			}

			// Normalize relaxed JSON: quote unquoted keys and convert single quotes
			normalized := normalizeRelaxedJSON(raw)
			if err := json.Unmarshal([]byte(normalized), &parsed); err == nil {
				for k, v := range parsed {
					result[k] = v
				}
				continue
			} else {
				return nil, fmt.Errorf("invalid json params %q: %w", raw, err)
			}
		}

		// Check for key=value
		if idx := strings.Index(raw, "="); idx > 0 {
			key := strings.TrimSpace(raw[:idx])
			valStr := strings.TrimSpace(raw[idx+1:])
			result[key] = parseScalarOrJSONValue(valStr)
			continue
		}

		return nil, fmt.Errorf("unrecognized param format: %q", raw)
	}
	return result, nil
}

func normalizeRelaxedJSON(s string) string {
	res := unquotedKeyRegex.ReplaceAllString(s, `$1"$2":`)
	return replaceSingleQuotes(res)
}

func replaceSingleQuotes(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && (i == 0 || s[i-1] != '\\') {
			sb.WriteByte('"')
		} else {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

func parseScalarOrJSONValue(val string) any {
	if val == "true" {
		return true
	}
	if val == "false" {
		return false
	}
	if val == "null" {
		return nil
	}
	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	if (strings.HasPrefix(val, "{") && strings.HasSuffix(val, "}")) ||
		(strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]")) {
		var j any
		if err := json.Unmarshal([]byte(val), &j); err == nil {
			return j
		}
		if err := json.Unmarshal([]byte(normalizeRelaxedJSON(val)), &j); err == nil {
			return j
		}
	}
	// Trim surrounding quotes if present
	if (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) ||
		(strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) {
		return val[1 : len(val)-1]
	}
	return val
}
