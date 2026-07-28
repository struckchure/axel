package pages

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/struckchure/axel/studio/db"
)

// StudioView is the full state rendered by the studio page and its partials.
type StudioView struct {
	ConnName string
	Live     bool

	HasSchema bool // an ASL schema is loaded (AQL available)
	AQLLive   bool // AQL executes against a live database

	Tables []db.Table // sidebar, ordered by schema then name
	Active *db.Table  // currently selected table, if any

	Tab     string // "data" | "structure" | "aql" | "sql"
	Rows    db.Rows
	Order   *db.Order
	DataErr string // non-empty when a data read failed (e.g. table not migrated)

	Query    *db.QueryResult
	QuerySQL string

	// LinkOptions holds picker candidates for each single-link column of the
	// active table, keyed by column name (editable views only).
	LinkOptions map[string][]db.Option
}

// Page returns the 1-based current page number.
func (v StudioView) Page() int {
	if v.Rows.Limit <= 0 {
		return 1
	}
	return v.Rows.Offset/v.Rows.Limit + 1
}

// Pages returns the total number of pages.
func (v StudioView) Pages() int {
	if v.Rows.Limit <= 0 {
		return 1
	}
	return int(math.Max(1, math.Ceil(float64(v.Rows.Total)/float64(v.Rows.Limit))))
}

// RangeLabel describes the visible row window, e.g. "1–50 of 1,204".
func (v StudioView) RangeLabel() string {
	if v.Rows.Total == 0 {
		return "0 rows"
	}
	from := v.Rows.Offset + 1
	to := v.Rows.Offset + len(v.Rows.Values)
	return fmt.Sprintf("%s–%s of %s", comma(int64(from)), comma(int64(to)), comma(v.Rows.Total))
}

// nav builds a "/?..." studio URL preserving the active table.
func (v StudioView) nav(tab string, extra map[string]string) string {
	q := url.Values{}
	if v.Active != nil {
		q.Set("schema", v.Active.Schema)
		q.Set("table", v.Active.Name)
	}
	if tab != "" {
		q.Set("tab", tab)
	}
	for k, val := range extra {
		if val == "" {
			q.Del(k)
			continue
		}
		q.Set(k, val)
	}
	return "/?" + q.Encode()
}

// tableURL links to a table's data view.
func tableURL(t db.Table) string {
	q := url.Values{}
	q.Set("schema", t.Schema)
	q.Set("table", t.Name)
	q.Set("tab", "data")
	return "/?" + q.Encode()
}

// sortURL toggles/sets ordering on a column and returns the studio URL.
func (v StudioView) sortURL(col string) string {
	desc := "false"
	if v.Order != nil && v.Order.Column == col && !v.Order.Desc {
		desc = "true"
	}
	return v.nav("data", map[string]string{"sort": col, "desc": desc})
}

func (v StudioView) sortIndicator(col string) string {
	if v.Order == nil || v.Order.Column != col {
		return ""
	}
	if v.Order.Desc {
		return "↓"
	}
	return "↑"
}

func (v StudioView) pageURL(page int) string {
	return v.nav("data", map[string]string{"page": fmt.Sprintf("%d", page)})
}

// Editable reports whether rows in the active table can be edited (live AQL +
// a schema-backed type).
func (v StudioView) Editable() bool {
	return v.AQLLive && v.Active != nil && v.Active.Type != ""
}

// isEditable reports whether a column's cells are inline-editable. Primary keys
// and links (relationships) are read-only for now.
func isEditable(c db.Column) bool {
	return !c.IsPrimaryKey && !c.IsLink
}

// pkIndex returns the index of the primary-key column, or -1.
func pkIndex(cols []db.Column) int {
	for i, c := range cols {
		if c.IsPrimaryKey {
			return i
		}
	}
	return -1
}

// pkValue returns the full (untruncated) primary-key value of a row as a string.
func pkValue(cols []db.Column, row []any) string {
	if i := pkIndex(cols); i >= 0 && i < len(row) {
		return editText(row[i])
	}
	return ""
}

// editText renders a value as plain, untruncated text for inline editing.
// NULLs become the empty string.
func editText(val any) string {
	if val == nil {
		return ""
	}
	c := formatCell(val)
	if c.Full != "" {
		return c.Full
	}
	if c.Kind == "null" {
		return ""
	}
	return c.Display
}

// SchemaGroup buckets tables under their schema for the sidebar.
type SchemaGroup struct {
	Schema string
	Tables []db.Table
}

func groupBySchema(tables []db.Table) []SchemaGroup {
	var groups []SchemaGroup
	idx := map[string]int{}
	for _, t := range tables {
		i, ok := idx[t.Schema]
		if !ok {
			i = len(groups)
			idx[t.Schema] = i
			groups = append(groups, SchemaGroup{Schema: t.Schema})
		}
		groups[i].Tables = append(groups[i].Tables, t)
	}
	return groups
}

func isActive(v StudioView, t db.Table) bool {
	return v.Active != nil && v.Active.Schema == t.Schema && v.Active.Name == t.Name
}

// Cell is a rendered table value with a kind used for styling.
type Cell struct {
	Display string
	Kind    string // null | bool | number | time | json | text
	Full    string // untruncated value for the title attribute
}

const cellMax = 80

func formatCell(v any) Cell {
	switch x := v.(type) {
	case nil:
		return Cell{Display: "NULL", Kind: "null"}
	case bool:
		return Cell{Display: fmt.Sprintf("%t", x), Kind: "bool"}
	case time.Time:
		return Cell{Display: x.Format("2006-01-02 15:04:05"), Kind: "time"}
	case [16]byte: // pgx decodes uuid to a raw byte array
		return Cell{Display: formatUUID(x), Kind: "text", Full: formatUUID(x)}
	case []byte:
		return truncate(string(x), "text")
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return Cell{Display: fmt.Sprintf("%d", x), Kind: "number"}
	case float32, float64:
		return Cell{Display: strings.TrimRight(fmt.Sprintf("%f", x), "0"), Kind: "number"}
	case string:
		return truncate(x, "text")
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return truncate(string(b), "json")
	default:
		return truncate(fmt.Sprintf("%v", x), "text")
	}
}

func truncate(s, kind string) Cell {
	full := s
	if len(s) > cellMax {
		s = s[:cellMax] + "…"
	}
	return Cell{Display: s, Kind: kind, Full: full}
}

// linkText renders a single-link value (its referenced id) as text.
func linkText(val any) string { return editText(val) }

// multiIDs converts a multi-link value ([]any of ids) to a string slice.
func multiIDs(val any) []string {
	arr, ok := val.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		out = append(out, editText(v))
	}
	return out
}

// optionLabel resolves an id to its picker label, falling back to the id.
func optionLabel(opts []db.Option, id string) string {
	for _, o := range opts {
		if o.ID == id {
			return o.Label
		}
	}
	return id
}

// linkCount returns the number of rows referenced by a multi link.
func linkCount(val any) int {
	if arr, ok := val.([]any); ok {
		return len(arr)
	}
	return 0
}

// linkTitle is the hover tooltip for a link cell: the referenced id(s).
func linkTitle(val any) string {
	if arr, ok := val.([]any); ok {
		parts := make([]string, len(arr))
		for i, v := range arr {
			parts[i] = editText(v)
		}
		return strings.Join(parts, ", ")
	}
	return editText(val)
}

func formatUUID(b [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func comma(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return "-" + comma(-n)
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
