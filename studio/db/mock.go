package db

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Mock is an in-memory Store used when no database is reachable, so the studio
// always has representative data to render. It models a small blog schema that
// mirrors axel's own example (User / Post).
type Mock struct {
	tables map[string]Table
	data   map[string][][]any
}

func NewMock() *Mock {
	base := time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)

	users := Table{Schema: "public", Name: "user", Rows: 6, Columns: []Column{
		{Name: "id", DataType: "uuid", IsPrimaryKey: true},
		{Name: "email", DataType: "text"},
		{Name: "name", DataType: "text"},
		{Name: "age", DataType: "int4"},
		{Name: "health", DataType: "int4", Default: "100"},
		{Name: "verified", DataType: "bool", Default: "false"},
		{Name: "created_at", DataType: "timestamptz", Default: "now()"},
	}}
	userRows := [][]any{
		{"a1c3…9f", "ada@axel.dev", "Ada Lovelace", 36, 100, true, base},
		{"b2d4…1a", "grace@axel.dev", "Grace Hopper", 41, 92, true, base.Add(48 * time.Hour)},
		{"c3e5…2b", "alan@axel.dev", "Alan Turing", 29, 100, false, base.Add(72 * time.Hour)},
		{"d4f6…3c", "linus@axel.dev", "Linus Torvalds", 33, 88, true, base.Add(96 * time.Hour)},
		{"e5a7…4d", "margaret@axel.dev", "Margaret Hamilton", 44, nil, false, base.Add(120 * time.Hour)},
		{"f6b8…5e", "dennis@axel.dev", "Dennis Ritchie", 38, 100, true, base.Add(144 * time.Hour)},
	}

	posts := Table{Schema: "public", Name: "post", Rows: 8, Columns: []Column{
		{Name: "id", DataType: "uuid", IsPrimaryKey: true},
		{Name: "title", DataType: "text"},
		{Name: "content", DataType: "text", Nullable: true},
		{Name: "published", DataType: "bool", Default: "false"},
		{Name: "views", DataType: "int4", Default: "0"},
		{Name: "author_id", DataType: "uuid", IsForeignKey: true, ForeignRef: "public.user.id"},
		{Name: "created_at", DataType: "timestamptz", Default: "now()"},
	}}
	postRows := [][]any{
		{"p1…01", "Notes on the Analytical Engine", "The engine weaves algebraic patterns…", true, 1240, "a1c3…9f", base.Add(2 * time.Hour)},
		{"p2…02", "Compilers for the rest of us", "A compiler is just a very patient friend.", true, 980, "b2d4…1a", base.Add(50 * time.Hour)},
		{"p3…03", "On computable numbers", nil, false, 0, "c3e5…2b", base.Add(74 * time.Hour)},
		{"p4…04", "Just for fun", "It started as a hobby…", true, 5400, "d4f6…3c", base.Add(98 * time.Hour)},
		{"p5…05", "Getting to the moon", "Every line of code carried people.", true, 3210, "e5a7…4d", base.Add(122 * time.Hour)},
		{"p6…06", "Unix philosophy", "Do one thing well.", true, 2750, "f6b8…5e", base.Add(146 * time.Hour)},
		{"p7…07", "Draft: distributed engines", nil, false, 0, "a1c3…9f", base.Add(170 * time.Hour)},
		{"p8…08", "Bletchley diaries", "Some days the machine wins.", false, 14, "c3e5…2b", base.Add(194 * time.Hour)},
	}

	tags := Table{Schema: "public", Name: "tag", Rows: 4, Columns: []Column{
		{Name: "id", DataType: "int4", IsPrimaryKey: true},
		{Name: "label", DataType: "text"},
		{Name: "color", DataType: "text", Nullable: true},
	}}
	tagRows := [][]any{
		{1, "history", "#f59e0b"},
		{2, "systems", "#3b82f6"},
		{3, "theory", "#8b5cf6"},
		{4, "hardware", nil},
	}

	return &Mock{
		tables: map[string]Table{
			"public.user": users,
			"public.post": posts,
			"public.tag":  tags,
		},
		data: map[string][][]any{
			"public.user": userRows,
			"public.post": postRows,
			"public.tag":  tagRows,
		},
	}
}

func (m *Mock) Name() string { return "sample data" }
func (m *Mock) Live() bool   { return false }

func (m *Mock) Tables(_ context.Context) ([]Table, error) {
	out := make([]Table, 0, len(m.tables))
	for _, t := range m.tables {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Schema != out[j].Schema {
			return out[i].Schema < out[j].Schema
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (m *Mock) Table(_ context.Context, schema, name string) (Table, error) {
	t, ok := m.tables[key(schema, name)]
	if !ok {
		return Table{}, fmt.Errorf("table %s.%s not found", schema, name)
	}
	return t, nil
}

func (m *Mock) Read(_ context.Context, schema, name string, limit, offset int, order *Order) (Rows, error) {
	t, ok := m.tables[key(schema, name)]
	if !ok {
		return Rows{}, fmt.Errorf("table %s.%s not found", schema, name)
	}
	all := m.data[key(schema, name)]

	if order != nil && order.Column != "" {
		if idx := colIndex(t, order.Column); idx >= 0 {
			sorted := make([][]any, len(all))
			copy(sorted, all)
			sort.SliceStable(sorted, func(i, j int) bool {
				less := lessAny(sorted[i][idx], sorted[j][idx])
				if order.Desc {
					return !less
				}
				return less
			})
			all = sorted
		}
	}

	total := int64(len(all))
	lo := min(offset, len(all))
	hi := min(offset+limit, len(all))
	return Rows{Columns: t.Columns, Values: all[lo:hi], Total: total, Offset: offset, Limit: limit}, nil
}

func (m *Mock) Query(_ context.Context, sql string) QueryResult {
	return QueryResult{
		Err: "sample-data mode: connect a live database to run SQL.\n\n— " + strings.TrimSpace(sql),
	}
}

func key(schema, name string) string { return schema + "." + name }

func colIndex(t Table, name string) int {
	for i, c := range t.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func lessAny(a, b any) bool {
	if a == nil {
		return b != nil
	}
	if b == nil {
		return false
	}
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			return av < bv
		}
	case time.Time:
		if bv, ok := b.(time.Time); ok {
			return av.Before(bv)
		}
	case bool:
		if bv, ok := b.(bool); ok {
			return !av && bv
		}
	}
	return fmt.Sprintf("%v", a) < fmt.Sprintf("%v", b)
}
