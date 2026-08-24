package repl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/struckchure/axel/core/runner"
)

// OutputFormat defines how query results are formatted.
type OutputFormat string

const (
	FormatPretty  OutputFormat = "pretty"
	FormatTable   OutputFormat = "table"
	FormatCompact OutputFormat = "compact"
)

// FormatResult formats query results according to the chosen OutputFormat.
func FormatResult(res *runner.Result, format OutputFormat) string {
	if res == nil {
		return "null"
	}

	if len(res.Rows) == 0 && res.RowsAffected > 0 {
		return fmt.Sprintf("{\"rows_affected\": %d}", res.RowsAffected)
	}

	switch format {
	case FormatCompact:
		b, err := json.Marshal(res.Rows)
		if err != nil {
			return fmt.Sprintf("error formatting compact JSON: %v", err)
		}
		return string(b)

	case FormatTable:
		return FormatTableRows(res.Rows)

	case FormatPretty:
		fallthrough
	default:
		b, err := json.MarshalIndent(res.Rows, "", "  ")
		if err != nil {
			return fmt.Sprintf("error formatting pretty JSON: %v", err)
		}
		return string(b)
	}
}

// FormatTableRows renders rows as an aligned ASCII table.
func FormatTableRows(rows []runner.Row) string {
	if len(rows) == 0 {
		return "(0 rows)"
	}

	// Collect all distinct column names.
	colSet := make(map[string]bool)
	var cols []string

	// Prioritize "id" as the first column if present in any row.
	hasID := false
	for _, r := range rows {
		if _, ok := r["id"]; ok {
			hasID = true
			break
		}
	}
	if hasID {
		cols = append(cols, "id")
		colSet["id"] = true
	}

	for _, r := range rows {
		var rowKeys []string
		for k := range r {
			if !colSet[k] {
				rowKeys = append(rowKeys, k)
			}
		}
		sort.Strings(rowKeys)
		for _, k := range rowKeys {
			cols = append(cols, k)
			colSet[k] = true
		}
	}

	// Prepare string representation of each cell.
	type cellRow []string
	var table [][]string
	colWidths := make([]int, len(cols))

	for i, c := range cols {
		colWidths[i] = len(c)
	}

	for _, r := range rows {
		rowVals := make([]string, len(cols))
		for i, col := range cols {
			val, exists := r[col]
			strVal := ""
			if !exists {
				strVal = "NULL"
			} else {
				strVal = formatCellValue(val)
			}
			rowVals[i] = strVal
			if len(strVal) > colWidths[i] {
				colWidths[i] = len(strVal)
			}
		}
		table = append(table, rowVals)
	}

	// Build separator line: +---------------+---------------+
	var sepParts []string
	for _, w := range colWidths {
		sepParts = append(sepParts, strings.Repeat("-", w+2))
	}
	sep := "+" + strings.Join(sepParts, "+") + "+"

	var b strings.Builder
	b.WriteString(sep + "\n")

	// Header row
	var headerParts []string
	for i, c := range cols {
		headerParts = append(headerParts, fmt.Sprintf(" %-*s ", colWidths[i], c))
	}
	b.WriteString("|" + strings.Join(headerParts, "|") + "|\n")
	b.WriteString(sep + "\n")

	// Data rows
	for _, row := range table {
		var rowParts []string
		for i, val := range row {
			rowParts = append(rowParts, fmt.Sprintf(" %-*s ", colWidths[i], val))
		}
		b.WriteString("|" + strings.Join(rowParts, "|") + "|\n")
	}
	b.WriteString(sep)

	return b.String()
}

func formatCellValue(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case map[string]any, []any:
		b, _ := json.Marshal(val)
		return string(b)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// FormatSummary formats a query completion summary line (e.g. "(3 rows in 2.45ms)").
func FormatSummary(res *runner.Result, duration time.Duration) string {
	durStr := formatDuration(duration)
	if res == nil {
		return fmt.Sprintf("(ok in %s)", durStr)
	}
	if len(res.Rows) > 0 {
		rowsWord := "rows"
		if len(res.Rows) == 1 {
			rowsWord = "row"
		}
		return fmt.Sprintf("(%d %s in %s)", len(res.Rows), rowsWord, durStr)
	}
	if res.RowsAffected > 0 {
		rowsWord := "rows"
		if res.RowsAffected == 1 {
			rowsWord = "row"
		}
		return fmt.Sprintf("(%d %s affected in %s)", res.RowsAffected, rowsWord, durStr)
	}
	return fmt.Sprintf("(0 rows in %s)", durStr)
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
