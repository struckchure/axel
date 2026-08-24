package lsp

import (
	"strings"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

// SchemaDefinition resolves a type reference under offset (a field annotation,
// `extending`, etc.) to its declaration in the same document. The returned
// Location has an empty URI, meaning "the current document" — the server fills
// it with the document's URI.
func SchemaDefinition(text string, offset int) *Location {
	return SchemaDefinitionIn(text, offset, nil)
}

// SchemaDefinitionIn resolves a type reference under offset in one file of a
// schema that may be split across several. The current document is searched
// first — a Location with an empty URI means "here" — and the sibling files
// after it, so a link whose target is declared in another file still resolves.
func SchemaDefinitionIn(text string, offset int, others []SchemaFile) *Location {
	word, start, _ := wordAt(text, offset)
	if word == "" {
		return nil
	}
	// Qualified enum member (EnumName.Value) resolves to the value token inside
	// the enum declaration, wherever that enum lives.
	if qualifier, ok := qualifierBefore(text, start); ok {
		if loc := enumValueLocation(qualifier, word, SchemaFile{Text: text}, others); loc != nil {
			return loc
		}
	}
	if rng, ok := schemaDeclRange(text, word); ok {
		return &Location{Range: rng}
	}
	if loc := declLocation(word, others); loc != nil {
		return loc
	}

	// Field reference inside ASL: e.g. `.created_at` in index/constraint/policy/rewrite,
	// or `on id` in a link declaration.
	allFiles := make([]SchemaFile, 0, len(others)+1)
	allFiles = append(allFiles, SchemaFile{Text: text})
	allFiles = append(allFiles, others...)

	if start > 0 && text[start-1] == '.' {
		if enclosing := enclosingTypeDef(text, offset); enclosing != "" {
			if loc := findFieldLocation(enclosing, word, allFiles); loc != nil {
				return loc
			}
		}
	}
	pw := prevWord(text, start)
	if pw == "on" {
		if target := linkTargetBefore(text, start); target != "" {
			if loc := findFieldLocation(target, word, allFiles); loc != nil {
				return loc
			}
		}
	}

	return nil
}

// QueryDefinition resolves the type name under offset in an AQL document to its
// declaration in the schema document (schemaURI/schemaText).
func QueryDefinition(text string, offset int, schema *asl.SchemaIR, schemaURI, schemaText string) *Location {
	return QueryDefinitionIn(text, offset, schema, []SchemaFile{{URI: schemaURI, Text: schemaText}})
}

// QueryDefinitionIn resolves the type name or field under offset in an AQL document to
// its declaration anywhere in the schema, which may span several files.
func QueryDefinitionIn(text string, offset int, schema *asl.SchemaIR, files []SchemaFile) *Location {
	if schema == nil || len(files) == 0 {
		return nil
	}

	// 1. Variable / Parameter reference ($param)
	if pName, _, _, isParam := paramNameAt(text, offset); isParam {
		if stmt, err := aql.ParseString(text); err == nil && stmt != nil {
			if stmt.For != nil {
				iter := strings.TrimPrefix(stmt.For.Iterator, "$")
				if iter == pName {
					return &Location{URI: "", Range: nameSelection(text, stmt.For.Pos, stmt.For.Iterator)}
				}
			}
			for _, v := range stmt.Vars {
				for _, p := range v.Params {
					if p.Name == pName {
						return &Location{URI: "", Range: nameSelection(text, p.Pos, "$"+p.Name)}
					}
				}
			}
		}
	}

	word, start, _ := wordAt(text, offset)
	if word == "" {
		return nil
	}

	// 2. With-binding, for-loop iterator, or var param by name (when clicking on binding name or param identifier)
	if stmt, err := aql.ParseString(text); err == nil && stmt != nil {
		if stmt.For != nil {
			iter := strings.TrimPrefix(stmt.For.Iterator, "$")
			if iter == word {
				return &Location{URI: "", Range: nameSelection(text, stmt.For.Pos, stmt.For.Iterator)}
			}
		}
		if stmt.With != nil {
			for _, b := range stmt.With.Bindings {
				if b.Name == word {
					return &Location{URI: "", Range: nameSelection(text, b.Pos, b.Name)}
				}
			}
		}
		for _, v := range stmt.Vars {
			for _, p := range v.Params {
				if p.Name == word {
					return &Location{URI: "", Range: nameSelection(text, p.Pos, "$"+p.Name)}
				}
			}
		}
	}

	// Qualified enum member (EnumName.Value): resolve to the value token inside
	// the enum declaration.
	if qualifier, ok := qualifierBefore(text, start); ok {
		if enum, known := schema.EnumTypes[qualifier]; known {
			for _, v := range enum.Values {
				if v == word {
					return enumValueLocation(qualifier, word, SchemaFile{}, files)
				}
			}
		}
	}

	// Top-level schema declarations: types, enums, scalars, functions, globals.
	if isSchemaTopLevel(word, schema) {
		return declLocation(word, files)
	}

	// Field / property references in AQL:
	rt := queryType(text, schema)
	if rt == nil {
		return nil
	}

	// Subfield of a link or typed JSON scalar via dot-path: e.g. in `.coord.lat` or `.author.name`
	if start > 0 && text[start-1] == '.' {
		pw := prevWord(text, start-1)
		if pw != "" {
			if prop, ok := rt.Properties[pw]; ok && prop.AQLType != "" {
				if loc := findFieldLocation(prop.AQLType, word, files); loc != nil {
					return loc
				}
			}
			if link, ok := rt.Links[pw]; ok && link.TargetType != "" {
				if loc := findFieldLocation(link.TargetType, word, files); loc != nil {
					return loc
				}
			}
		}
		// Single dot field: `.field`
		return findFieldLocation(rt.Name, word, files)
	}

	// Field inside shape or update/insert set body:
	if targetType := shapeFieldType(text, offset, schema); targetType != "" {
		return findFieldLocation(targetType, word, files)
	}

	return findFieldLocation(rt.Name, word, files)
}

func isSchemaTopLevel(name string, schema *asl.SchemaIR) bool {
	if _, ok := schema.ObjectTypes[name]; ok {
		return true
	}
	if _, ok := schema.EnumTypes[name]; ok {
		return true
	}
	if _, ok := schema.ScalarTypes[name]; ok {
		return true
	}
	if _, ok := schema.Functions[name]; ok {
		return true
	}
	for _, g := range schema.Globals {
		if g.Name == name {
			return true
		}
	}
	return false
}

// qualifierBefore returns the identifier immediately preceding a `.` at start,
// e.g. "Role" for the cursor inside `Role.Admin`.
func qualifierBefore(text string, start int) (string, bool) {
	if start < 2 || text[start-1] != '.' {
		return "", false
	}
	word, _, _ := wordAt(text, start-2)
	return word, word != ""
}

// declLocation returns the first file declaring name, and the range of its name
// token. Files are searched in order, so the primary schema wins a tie.
func declLocation(name string, files []SchemaFile) *Location {
	for _, f := range files {
		if rng, ok := schemaDeclRange(f.Text, name); ok {
			return &Location{URI: f.URI, Range: rng}
		}
	}
	return nil
}

// enumValueLocation finds the token for value inside the declaration of the enum
// named enumName. current (when it has text) is searched before files, and a
// match there yields an empty URI, meaning the current document.
func enumValueLocation(enumName, value string, current SchemaFile, files []SchemaFile) *Location {
	if current.Text != "" {
		if rng, ok := enumValueDeclRange(current.Text, enumName, value); ok {
			return &Location{URI: current.URI, Range: rng}
		}
	}
	for _, f := range files {
		if rng, ok := enumValueDeclRange(f.Text, enumName, value); ok {
			return &Location{URI: f.URI, Range: rng}
		}
	}
	return nil
}

// enumValueDeclRange returns the range of value's token inside the declaration
// of the enum named enumName, in schemaText.
func enumValueDeclRange(schemaText, enumName, value string) (Range, bool) {
	sf, err := asl.Parse([]byte(schemaText))
	if err != nil || sf == nil {
		return Range{}, false
	}
	for _, d := range sf.Definitions {
		if d.EnumType == nil || d.EnumType.Name != enumName {
			continue
		}
		e := d.EnumType
		from := indexWord(schemaText, e.Name, e.Pos.Offset)
		if from < 0 {
			from = e.Pos.Offset
		} else {
			from += len(e.Name)
		}
		idx := indexWord(schemaText, value, from)
		if idx < 0 || (e.EndPos.Offset > 0 && idx >= e.EndPos.Offset) {
			return Range{}, false
		}
		return Range{
			Start: OffsetToPosition(schemaText, idx),
			End:   OffsetToPosition(schemaText, idx+len(value)),
		}, true
	}
	return Range{}, false
}

// schemaDeclRange parses an ASL document and returns the name-token range of the
// top-level declaration (type/enum/scalar/function/global) named name.
func schemaDeclRange(schemaText, name string) (Range, bool) {
	sf, err := asl.Parse([]byte(schemaText))
	if err != nil || sf == nil {
		return Range{}, false
	}
	for _, d := range sf.Definitions {
		switch {
		case d.TypeDef != nil && d.TypeDef.Name == name:
			return nameSelection(schemaText, d.TypeDef.Pos, name), true
		case d.EnumType != nil && d.EnumType.Name == name:
			return nameSelection(schemaText, d.EnumType.Pos, name), true
		case d.ScalarType != nil && d.ScalarType.Name == name:
			return nameSelection(schemaText, d.ScalarType.Pos, name), true
		case d.Function != nil && d.Function.Name == name:
			return nameSelection(schemaText, d.Function.Pos, name), true
		case d.Global != nil && d.Global.Name == name:
			return nameSelection(schemaText, d.Global.Pos, name), true
		}
	}
	return Range{}, false
}

// findFieldLocation finds a field/property/link in typeName or its parent types across files.
func findFieldLocation(typeName, fieldName string, files []SchemaFile) *Location {
	return searchFieldInHierarchy(typeName, fieldName, files, map[string]bool{})
}

func searchFieldInHierarchy(typeName, fieldName string, files []SchemaFile, visited map[string]bool) *Location {
	if visited[typeName] {
		return nil
	}
	visited[typeName] = true

	var parents []string
	for _, f := range files {
		sf, err := asl.Parse([]byte(f.Text))
		if err != nil || sf == nil {
			continue
		}
		for _, d := range sf.Definitions {
			if d.TypeDef != nil && d.TypeDef.Name == typeName {
				for _, m := range d.TypeDef.Members {
					if m.Field != nil && m.Field.Name == fieldName {
						return &Location{URI: f.URI, Range: nameSelection(f.Text, m.Field.Pos, fieldName)}
					}
					if m.Computed != nil && m.Computed.Name == fieldName {
						return &Location{URI: f.URI, Range: nameSelection(f.Text, m.Computed.Pos, fieldName)}
					}
				}
				parents = append(parents, d.TypeDef.Extending...)
			} else if d.ScalarType != nil && d.ScalarType.Name == typeName {
				if d.ScalarType.Body != nil {
					for _, fld := range d.ScalarType.Body.Fields {
						if fld.Name == fieldName {
							return &Location{URI: f.URI, Range: nameSelection(f.Text, fld.Pos, fieldName)}
						}
					}
				}
				if d.ScalarType.Extends != "" {
					parents = append(parents, d.ScalarType.Extends)
				}
			}
		}
	}

	for _, p := range parents {
		if loc := searchFieldInHierarchy(p, fieldName, files, visited); loc != nil {
			return loc
		}
	}
	return nil
}

// enclosingTypeDef returns the name of the TypeDef enclosing offset in ASL text.
func enclosingTypeDef(text string, offset int) string {
	sf, err := asl.Parse([]byte(text))
	if err != nil || sf == nil {
		return ""
	}
	for _, d := range sf.Definitions {
		if d.TypeDef != nil {
			start := d.TypeDef.Pos.Offset
			end := d.TypeDef.EndPos.Offset
			if end == 0 {
				end = len(text)
			}
			if offset >= start && offset <= end {
				return d.TypeDef.Name
			}
		}
	}
	return ""
}

// linkTargetBefore inspects the text before offset for a link declaration and returns the target type name.
func linkTargetBefore(text string, offset int) string {
	lineStart := strings.LastIndex(text[:offset], "\n")
	if lineStart < 0 {
		lineStart = 0
	}
	line := text[lineStart:offset]
	colonIdx := strings.Index(line, ":")
	if colonIdx < 0 {
		return ""
	}
	parts := strings.Fields(line[colonIdx+1:])
	if len(parts) > 0 {
		return strings.Trim(parts[0], ";{ \t")
	}
	return ""
}

// shapeFieldType determines the target object type enclosing offset inside an AQL shape.
func shapeFieldType(text string, offset int, schema *asl.SchemaIR) string {
	stmt, err := aql.ParseString(text)
	if err != nil || stmt == nil {
		return ""
	}
	_, typeName := stmtInfo(stmt)
	if typeName == "" {
		return ""
	}
	if stmt.Select != nil && stmt.Select.Body != nil && stmt.Select.Body.Shape != nil {
		if target := findShapeTargetType(stmt.Select.Body.Shape, typeName, schema, offset); target != "" {
			return target
		}
	}
	return typeName
}

func findShapeTargetType(shape *aql.Shape, currentType string, schema *asl.SchemaIR, offset int) string {
	rt := schema.ObjectTypes[currentType]
	if rt == nil {
		return currentType
	}
	for _, f := range shape.Fields {
		if f.SubShape != nil && len(f.SubShape.Fields) > 0 {
			subStart := f.SubShape.Fields[0].Pos.Offset
			if offset >= subStart {
				if link, ok := rt.Links[f.Name]; ok {
					return findShapeTargetType(f.SubShape, link.TargetType, schema, offset)
				}
			}
		}
	}
	return currentType
}

