package db

import (
	"os"
	"sort"

	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/codegen"
)

// Schema is a loaded, resolved ASL schema. It powers the schema-driven sidebar
// and structure views and backs AQL execution.
type Schema struct {
	IR   *asl.SchemaIR
	Desc codegen.SchemaDescriptor
	Path string

	byType  map[string]codegen.TypeDescriptor
	byTable map[string]codegen.TypeDescriptor
}

// LoadSchema reads and resolves an .asl file.
func LoadSchema(path string) (*Schema, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sf, err := asl.Parse(src)
	if err != nil {
		return nil, err
	}
	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		return nil, err
	}

	s := &Schema{
		IR:      ir,
		Desc:    codegen.FromSchemaIR(ir),
		Path:    path,
		byType:  map[string]codegen.TypeDescriptor{},
		byTable: map[string]codegen.TypeDescriptor{},
	}
	for _, t := range s.Desc.Types {
		s.byType[t.Name] = t
		if t.Table != "" {
			s.byTable[t.Table] = t
		}
	}
	return s, nil
}

// TypeByTable resolves an ASL type from its backing table name.
func (s *Schema) TypeByTable(table string) (codegen.TypeDescriptor, bool) {
	t, ok := s.byTable[table]
	return t, ok
}

// Option is one selectable value in a link picker: a referenced row's id and a
// human-readable label.
type Option struct {
	ID    string
	Label string
}

// SingleLinkTarget returns the target type of a single (non-multi) link field.
func (s *Schema) SingleLinkTarget(typeName, field string) (target string, ok bool) {
	t, found := s.byType[typeName]
	if !found {
		return "", false
	}
	for _, l := range t.Links {
		if l.Name == field && !l.IsMulti {
			return l.TargetType, true
		}
	}
	return "", false
}

// IsMultiLink reports whether a field is a multi link.
func (s *Schema) IsMultiLink(typeName, field string) bool {
	_, ok := s.MultiLinkTarget(typeName, field)
	return ok
}

// MultiLinkTarget returns the target type of a multi link field.
func (s *Schema) MultiLinkTarget(typeName, field string) (target string, ok bool) {
	t, found := s.byType[typeName]
	if !found {
		return "", false
	}
	for _, l := range t.Links {
		if l.Name == field && l.IsMulti {
			return l.TargetType, true
		}
	}
	return "", false
}

// labelField picks a human-friendly property to label rows of a type in a link
// picker, preferring common name-like fields, then any non-id string property,
// falling back to "id".
func (s *Schema) labelField(typeName string) string {
	t, found := s.byType[typeName]
	if !found {
		return "id"
	}
	has := map[string]bool{}
	for _, p := range t.Properties {
		has[p.Name] = true
	}
	for _, pref := range []string{"name", "title", "label", "slug", "email", "username"} {
		if has[pref] {
			return pref
		}
	}
	for _, p := range t.Properties {
		if p.Name != "id" && p.AQLType == "str" {
			return p.Name
		}
	}
	return "id"
}

// ColumnType returns the AQL type and nullability of a type's scalar property.
func (s *Schema) ColumnType(typeName, col string) (aqlType string, nullable bool, ok bool) {
	t, found := s.byType[typeName]
	if !found {
		return "", false, false
	}
	for _, p := range t.Properties {
		if p.Name == col {
			return p.AQLType, !p.IsRequired, true
		}
	}
	return "", false, false
}

// Tables renders concrete (non-abstract) types as studio tables, ordered by name.
// Abstract types are omitted — they back no table of their own.
func (s *Schema) Tables() []Table {
	var out []Table
	for _, t := range s.Desc.Types {
		if t.IsAbstract || t.Table == "" {
			continue
		}
		out = append(out, typeToTable(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Table renders a single type as a studio table.
func (s *Schema) Table(name string) (Table, bool) {
	t, ok := s.byTable[name]
	if !ok || t.IsAbstract {
		return Table{}, false
	}
	return typeToTable(t), true
}

func typeToTable(t codegen.TypeDescriptor) Table {
	tbl := Table{Schema: "public", Name: t.Table, Type: t.Name}

	props := append([]codegen.PropertyDescriptor(nil), t.Properties...)
	sort.SliceStable(props, func(i, j int) bool {
		pi, pj := isPK(props[i]), isPK(props[j])
		if pi != pj {
			return pi // primary keys first
		}
		return props[i].Name < props[j].Name
	})
	for _, p := range props {
		tbl.Columns = append(tbl.Columns, Column{
			Name:         p.Name,
			DataType:     p.AQLType,
			SQLType:      p.SQLType,
			Nullable:     !p.IsRequired,
			Default:      p.Default,
			IsPrimaryKey: isPK(p),
		})
	}
	for _, l := range t.Links {
		c := Column{
			Name:       l.Name,
			DataType:   linkType(l),
			Nullable:   !l.IsRequired,
			IsLink:     true,
			IsMulti:    l.IsMulti,
			ForeignRef: l.TargetType,
		}
		if !l.IsMulti {
			c.IsForeignKey = true
		}
		tbl.Columns = append(tbl.Columns, c)
	}
	return tbl
}

// gridColumns returns the columns rendered in the row grid and selected by the
// read query: all scalars plus links. Single links resolve to their referenced
// row; multi links resolve to a JSON array of referenced rows.
func gridColumns(t Table) []Column {
	return t.Columns
}

func isPK(p codegen.PropertyDescriptor) bool {
	for _, c := range p.Constraints {
		if c.Name == "pk" {
			return true
		}
	}
	return false
}

func linkType(l codegen.LinkDescriptor) string {
	if l.IsMulti {
		return "multi link → " + l.TargetType
	}
	return "link → " + l.TargetType
}
