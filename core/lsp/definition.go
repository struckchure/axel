package lsp

import "github.com/struckchure/axel/core/asl"

// SchemaDefinition resolves a type reference under offset (a field annotation,
// `extending`, etc.) to its declaration in the same document. The returned
// Location has an empty URI, meaning "the current document" — the server fills
// it with the document's URI.
func SchemaDefinition(text string, offset int) *Location {
	word, _, _ := wordAt(text, offset)
	if word == "" {
		return nil
	}
	if rng, ok := schemaDeclRange(text, word); ok {
		return &Location{Range: rng}
	}
	return nil
}

// QueryDefinition resolves the type name under offset in an AQL document to its
// declaration in the schema document (schemaURI/schemaText).
func QueryDefinition(text string, offset int, schema *asl.SchemaIR, schemaURI, schemaText string) *Location {
	if schema == nil || schemaText == "" {
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
	if start >= 2 && text[start-1] == '.' {
		if qualifier, _, _ := wordAt(text, start-2); qualifier != "" {
			if enum, ok := schema.EnumTypes[qualifier]; ok {
				for _, v := range enum.Values {
					if v == word {
						if rng, ok := enumValueDeclRange(schemaText, qualifier, word); ok {
							return &Location{URI: schemaURI, Range: rng}
						}
						return nil
					}
				}
			}
		}
	}
	_, isType := schema.ObjectTypes[word]
	_, isEnum := schema.EnumTypes[word]
	if !isType && !isEnum {
		return nil
	}
	if rng, ok := schemaDeclRange(schemaText, word); ok {
		return &Location{URI: schemaURI, Range: rng}
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
