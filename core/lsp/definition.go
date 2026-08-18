package lsp

import "github.com/struckchure/axel/core/asl"

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
	return declLocation(word, others)
}

// QueryDefinition resolves the type name under offset in an AQL document to its
// declaration in the schema document (schemaURI/schemaText).
func QueryDefinition(text string, offset int, schema *asl.SchemaIR, schemaURI, schemaText string) *Location {
	return QueryDefinitionIn(text, offset, schema, []SchemaFile{{URI: schemaURI, Text: schemaText}})
}

// QueryDefinitionIn resolves the type name under offset in an AQL document to
// its declaration anywhere in the schema, which may span several files. schema
// is the resolved IR of all of them, and decides whether the name is a type at
// all; files are then searched in order for the one that declares it.
func QueryDefinitionIn(text string, offset int, schema *asl.SchemaIR, files []SchemaFile) *Location {
	if schema == nil || len(files) == 0 {
		return nil
	}
	word, start, _ := wordAt(text, offset)
	if word == "" {
		return nil
	}
	// Qualified enum member (EnumName.Value): resolve to the value token inside
	// the enum declaration, not to a same-named top-level type. This must run
	// before the plain type/enum lookup so `TransactionActorEntity.ApiKey` does
	// not jump to a `type ApiKey` that happens to share the value's name.
	if qualifier, ok := qualifierBefore(text, start); ok {
		if enum, known := schema.EnumTypes[qualifier]; known {
			for _, v := range enum.Values {
				if v == word {
					return enumValueLocation(qualifier, word, SchemaFile{}, files)
				}
			}
		}
	}
	_, isType := schema.ObjectTypes[word]
	_, isEnum := schema.EnumTypes[word]
	if !isType && !isEnum {
		return nil
	}
	return declLocation(word, files)
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
		// Start the search past the enum's name so a value that repeats the enum
		// name is not shadowed by the name token itself.
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
// top-level declaration (type/enum/scalar) named name.
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
		}
	}
	return Range{}, false
}
