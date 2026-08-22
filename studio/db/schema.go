package db

import (
	"sort"

	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/codegen"
)

// Schema is a loaded, resolved ASL schema. It powers the schema-driven sidebar
// and structure views and backs AQL execution.
type Schema struct {
	IR    *asl.SchemaIR
	Desc  codegen.SchemaDescriptor
	Path  string   // the path spec the schema was loaded from (file, dir or glob)
	Files []string // the .asl files it expanded to

	byType  map[string]codegen.TypeDescriptor
	byTable map[string]codegen.TypeDescriptor
}

// LoadSchema reads and resolves a schema. path may name a single .asl file, a
// directory, or a glob matching several files, which are merged into one schema.
func LoadSchema(path string) (*Schema, error) {
	ir, files, err := asl.LoadIR(path)
	if err != nil {
		return nil, err
	}

	s := &Schema{
		IR:      ir,
		Desc:    codegen.FromSchemaIR(ir),
		Path:    path,
		Files:   files,
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

// EnumItem describes an enum definition in the schema.
type EnumItem struct {
	Name   string      `json:"name"`
	Values []string    `json:"values"`
	Usages []EnumUsage `json:"usages"`
}

// EnumUsage records a type and property referencing an enum.
type EnumUsage struct {
	Type     string `json:"type"`
	Property string `json:"property"`
}

// ScalarItem describes a custom scalar definition in the schema.
type ScalarItem struct {
	Name          string                   `json:"name"`
	Base          string                   `json:"base"`
	SQLType       string                   `json:"sql_type"`
	Fields        []ScalarFieldItem        `json:"fields"`
	ExtendKeyword string                   `json:"extend_keyword"`
	Default       string                   `json:"default"`
	Constraints   []asl.ResolvedConstraint `json:"constraints"`
	Rewrites      []asl.ResolvedRewrite    `json:"rewrites"`
	Usages        []ScalarUsage            `json:"usages"`
}

// ScalarFieldItem describes a field in a typed JSON scalar.
type ScalarFieldItem struct {
	Name       string `json:"name"`
	AQLType    string `json:"aql_type"`
	SQLType    string `json:"sql_type"`
	IsRequired bool   `json:"is_required"`
	IsMulti    bool   `json:"is_multi"`
}

// ScalarUsage records a type and property referencing a custom scalar.
type ScalarUsage struct {
	Type     string `json:"type"`
	Property string `json:"property"`
}

// ExtensionItem describes a Postgres extension declared with `use extension`.
type ExtensionItem struct {
	Name string `json:"name"`
}

// FunctionItem describes a top-level Postgres function declared in the schema.
type FunctionItem struct {
	Name       string          `json:"name"`
	Params     []FunctionParam `json:"params"`
	Returns    string          `json:"returns"`
	Language   string          `json:"language"`
	ReturnSQL  string          `json:"return_sql"`
	Volatility string          `json:"volatility"`
	Strict     bool            `json:"strict"`
	Leakproof  bool            `json:"leakproof"`
	Parallel   string          `json:"parallel"`
	Security   string          `json:"security"`
	Cost       string          `json:"cost"`
	RunOnceFor string          `json:"run_once_for"`
	Usages     []string        `json:"usages"`
}

// FunctionParam describes one parameter of a function.
type FunctionParam struct {
	Name    string `json:"name"`
	SQLType string `json:"sql_type"`
}

// PolicyItem describes an RLS policy attached to an ASL type.
type PolicyItem struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Table    string   `json:"table"`
	Commands []string `json:"commands"`
	Roles    []string `json:"roles"`
	UsingAQL string   `json:"using_aql"`
	CheckAQL string   `json:"check_aql"`
}

// TriggerItem describes a trigger attached to an ASL type.
type TriggerItem struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Table    string   `json:"table"`
	Timing   string   `json:"timing"`
	Events   []string `json:"events"`
	ForEach  string   `json:"for_each"`
	When     string   `json:"when"`
	DoAQL    string   `json:"do_aql"`
	Function string   `json:"function"`
}

// Enums extracts all enum types sorted by name, along with their schema usages.
func (s *Schema) Enums() []EnumItem {
	if s.IR == nil || len(s.IR.EnumTypes) == 0 {
		return nil
	}
	var out []EnumItem
	for name, enum := range s.IR.EnumTypes {
		var usages []EnumUsage
		for _, t := range s.IR.ObjectTypes {
			for _, p := range t.Properties {
				if p.EnumType == name || p.AQLType == name {
					usages = append(usages, EnumUsage{Type: t.Name, Property: p.Name})
				}
			}
		}
		sort.Slice(usages, func(i, j int) bool {
			if usages[i].Type != usages[j].Type {
				return usages[i].Type < usages[j].Type
			}
			return usages[i].Property < usages[j].Property
		})
		out = append(out, EnumItem{
			Name:   name,
			Values: enum.Values,
			Usages: usages,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Scalars extracts all custom scalar types sorted by name.
func (s *Schema) Scalars() []ScalarItem {
	if s.IR == nil || len(s.IR.ScalarTypes) == 0 {
		return nil
	}
	var out []ScalarItem
	for name, sc := range s.IR.ScalarTypes {
		var fields []ScalarFieldItem
		for _, f := range sc.Fields {
			fields = append(fields, ScalarFieldItem{
				Name:       f.Name,
				AQLType:    f.AQLType,
				SQLType:    f.SQLType,
				IsRequired: f.IsRequired,
				IsMulti:    f.IsMulti,
			})
		}
		sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })

		var usages []ScalarUsage
		for _, t := range s.IR.ObjectTypes {
			for _, p := range t.Properties {
				if p.AQLType == name {
					usages = append(usages, ScalarUsage{Type: t.Name, Property: p.Name})
				}
			}
		}
		sort.Slice(usages, func(i, j int) bool {
			if usages[i].Type != usages[j].Type {
				return usages[i].Type < usages[j].Type
			}
			return usages[i].Property < usages[j].Property
		})

		out = append(out, ScalarItem{
			Name:          name,
			Base:          sc.Base,
			SQLType:       sc.SQLType,
			Fields:        fields,
			ExtendKeyword: sc.ExtendKeyword,
			Default:       sc.Default,
			Constraints:   sc.Constraints,
			Rewrites:      sc.Rewrites,
			Usages:        usages,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Policies extracts all RLS policies across all types in the schema.
func (s *Schema) Policies() []PolicyItem {
	if s.IR == nil || len(s.IR.ObjectTypes) == 0 {
		return nil
	}
	var out []PolicyItem
	for _, t := range s.IR.ObjectTypes {
		for _, p := range t.Policies {
			out = append(out, PolicyItem{
				Name:     p.Name,
				Type:     t.Name,
				Table:    t.Table,
				Commands: p.Commands,
				Roles:    p.Roles,
				UsingAQL: p.UsingAQL,
				CheckAQL: p.CheckAQL,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Triggers extracts all triggers across all types in the schema.
func (s *Schema) Triggers() []TriggerItem {
	if s.IR == nil || len(s.IR.ObjectTypes) == 0 {
		return nil
	}
	var out []TriggerItem
	for _, t := range s.IR.ObjectTypes {
		for _, tr := range t.Triggers {
			out = append(out, TriggerItem{
				Name:     tr.Name,
				Type:     t.Name,
				Table:    t.Table,
				Timing:   tr.Timing,
				Events:   tr.Events,
				ForEach:  tr.ForEach,
				When:     tr.When,
				DoAQL:    tr.DoAQL,
				Function: tr.Function,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Extensions extracts all declared Postgres extensions.
func (s *Schema) Extensions() []ExtensionItem {
	if s.IR == nil || len(s.IR.Extensions) == 0 {
		return nil
	}
	var out []ExtensionItem
	for _, ext := range s.IR.Extensions {
		out = append(out, ExtensionItem{Name: ext})
	}
	return out
}

// Functions extracts all declared top-level functions sorted by name.
func (s *Schema) Functions() []FunctionItem {
	if s.IR == nil || len(s.IR.Functions) == 0 {
		return nil
	}
	var out []FunctionItem
	for name, fn := range s.IR.Functions {
		var params []FunctionParam
		for _, p := range fn.Params {
			params = append(params, FunctionParam{
				Name:    p.Name,
				SQLType: p.SQLType,
			})
		}
		var usages []string
		for _, t := range s.IR.ObjectTypes {
			for _, tr := range t.Triggers {
				if tr.Function == name {
					usages = append(usages, t.Name+"."+tr.Name)
				}
			}
		}
		sort.Strings(usages)

		out = append(out, FunctionItem{
			Name:       name,
			Params:     params,
			Returns:    fn.Returns,
			Language:   fn.Language,
			ReturnSQL:  fn.ReturnSQL,
			Volatility: fn.Volatility,
			Strict:     fn.Strict,
			Leakproof:  fn.Leakproof,
			Parallel:   fn.Parallel,
			Security:   fn.Security,
			Cost:       fn.Cost,
			RunOnceFor: fn.RunOnceFor,
			Usages:     usages,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
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
		out = append(out, s.typeToTable(t))
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
	return s.typeToTable(t), true
}

func (s *Schema) typeToTable(t codegen.TypeDescriptor) Table {
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
		enumType := p.EnumType
		var enumValues []string
		if enumType == "" && s.IR != nil {
			if _, isEnum := s.IR.EnumTypes[p.AQLType]; isEnum {
				enumType = p.AQLType
			}
		}
		if enumType != "" {
			if s.IR != nil && s.IR.EnumTypes[enumType] != nil {
				enumValues = s.IR.EnumTypes[enumType].Values
			} else {
				for _, e := range s.Desc.Enums {
					if e.Name == enumType {
						enumValues = e.Values
						break
					}
				}
			}
		}

		tbl.Columns = append(tbl.Columns, Column{
			Name:         p.Name,
			DataType:     p.AQLType,
			SQLType:      p.SQLType,
			Nullable:     !p.IsRequired,
			Default:      p.Default,
			IsPrimaryKey: isPK(p),
			EnumType:     enumType,
			EnumValues:   enumValues,
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
