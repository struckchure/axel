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
	word, _, _ := wordAt(text, offset)
	if word == "" {
		return nil
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
