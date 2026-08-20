package lsp

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/asl"
)

func offsetIn(t *testing.T, text, needle string) int {
	t.Helper()
	i := strings.Index(text, needle)
	if i < 0 {
		t.Fatalf("%q not found in document", needle)
	}
	return i + 1 // inside the token
}

// resolveAll merges several documents the way the server does, so a definition
// lookup can be checked against the schema the editor actually sees.
func resolveAll(t *testing.T, files ...SchemaFile) *asl.SchemaIR {
	t.Helper()
	var parsed []*asl.SourceFile
	for _, f := range files {
		sf, err := asl.ParseNamed(f.Path, []byte(f.Text))
		if err != nil {
			t.Fatalf("parse %s: %v", f.Path, err)
		}
		parsed = append(parsed, sf)
	}
	ir, err := (&asl.Resolver{}).Resolve(asl.Merge(parsed...))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return ir
}

const (
	userASL = "enum Role { Admin, Member }\n\ntype User {\n  required email: str;\n  required role: Role;\n}\n"
	bookASL = "type Book {\n  required title: str;\n  required link author: User;\n}\n"
)

var userFile = SchemaFile{Path: "user.asl", URI: "file:///user.asl", Text: userASL}

// A link target declared in a sibling file resolves to that file.
func TestSchemaDefinitionAcrossFiles(t *testing.T) {
	loc := SchemaDefinitionIn(bookASL, offsetIn(t, bookASL, "User;"), []SchemaFile{userFile})
	if loc == nil {
		t.Fatal("no definition found for a type declared in a sibling file")
	}
	if loc.URI != userFile.URI {
		t.Errorf("URI = %q, want %q", loc.URI, userFile.URI)
	}
	if got := lineOf(userASL, loc.Range.Start.Line); !strings.Contains(got, "type User") {
		t.Errorf("landed on %q, want the `type User` line", got)
	}
}

// A reference to a type declared in the same document still reports "here" —
// an empty URI the server fills in with the document's own.
func TestSchemaDefinitionPrefersCurrentDocument(t *testing.T) {
	local := "type User { required name: str; }\ntype Book { required link author: User; }\n"
	loc := SchemaDefinitionIn(local, offsetIn(t, local, "User; }"), []SchemaFile{userFile})
	if loc == nil {
		t.Fatal("no definition found")
	}
	if loc.URI != "" {
		t.Errorf("URI = %q, want empty (current document)", loc.URI)
	}
}

// An enum member qualified by an enum declared elsewhere lands on the member.
func TestSchemaDefinitionEnumMemberAcrossFiles(t *testing.T) {
	doc := "type Account {\n  required role: Role { default := Role.Admin };\n}\n"
	loc := SchemaDefinitionIn(doc, offsetIn(t, doc, "Admin };"), []SchemaFile{userFile})
	if loc == nil {
		t.Fatal("no definition found for an enum member declared in a sibling file")
	}
	if loc.URI != userFile.URI {
		t.Errorf("URI = %q, want %q", loc.URI, userFile.URI)
	}
	if got := lineOf(userASL, loc.Range.Start.Line); !strings.Contains(got, "enum Role") {
		t.Errorf("landed on %q, want the enum declaration line", got)
	}
}

// A query jumps into whichever schema file declares the type.
func TestQueryDefinitionAcrossFiles(t *testing.T) {
	bookFile := SchemaFile{Path: "book.asl", URI: "file:///book.asl", Text: bookASL}
	files := []SchemaFile{userFile, bookFile}
	schema := resolveAll(t, files...)

	q := "multi select Book { title };"
	loc := QueryDefinitionIn(q, offsetIn(t, q, "Book {"), schema, files)
	if loc == nil {
		t.Fatal("no definition found for Book")
	}
	if loc.URI != bookFile.URI {
		t.Errorf("URI = %q, want %q", loc.URI, bookFile.URI)
	}

	q2 := "multi select User { email } filter .role = Role.Admin;"
	loc2 := QueryDefinitionIn(q2, offsetIn(t, q2, "Admin;"), schema, files)
	if loc2 == nil || loc2.URI != userFile.URI {
		t.Fatalf("enum member = %+v, want a location in %s", loc2, userFile.URI)
	}
}

// Functions, globals, scalars, and inherited fields resolve across files in ASL and AQL.
func TestAdvancedDefinitionAcrossFiles(t *testing.T) {
	baseASL := "abstract type Base {\n  required id: uuid;\n  created_at: datetime;\n}\n"
	funcASL := "function audit_log() -> trigger { return NEW; };\n"
	globalASL := "global required tenant_id: uuid;\n"
	scalarASL := "scalar type Code extends str {\n  constraint min_length(6);\n}\n"
	modelASL := `type Document extends Base {
  required title: str;
  code: Code;
  trigger audit after insert execute audit_log();
  policy tenant_isolation for all using ( .id = tenant_id );
  index on (.title, .created_at);
}
`
	baseFile := SchemaFile{Path: "base.asl", URI: "file:///base.asl", Text: baseASL}
	funcFile := SchemaFile{Path: "func.asl", URI: "file:///func.asl", Text: funcASL}
	globalFile := SchemaFile{Path: "global.asl", URI: "file:///global.asl", Text: globalASL}
	scalarFile := SchemaFile{Path: "scalar.asl", URI: "file:///scalar.asl", Text: scalarASL}
	modelFile := SchemaFile{Path: "model.asl", URI: "file:///model.asl", Text: modelASL}

	files := []SchemaFile{baseFile, funcFile, globalFile, scalarFile, modelFile}
	schema := resolveAll(t, files...)

	// ASL: execute audit_log() -> func.asl
	loc := SchemaDefinitionIn(modelASL, offsetIn(t, modelASL, "audit_log()"), files)
	if loc == nil || loc.URI != funcFile.URI {
		t.Fatalf("audit_log loc = %+v, want %s", loc, funcFile.URI)
	}

	// ASL: tenant_id -> global.asl
	loc = SchemaDefinitionIn(modelASL, offsetIn(t, modelASL, "tenant_id"), files)
	if loc == nil || loc.URI != globalFile.URI {
		t.Fatalf("tenant_id loc = %+v, want %s", loc, globalFile.URI)
	}

	// ASL: Code -> scalar.asl
	loc = SchemaDefinitionIn(modelASL, offsetIn(t, modelASL, "Code;"), files)
	if loc == nil || loc.URI != scalarFile.URI {
		t.Fatalf("Code loc = %+v, want %s", loc, scalarFile.URI)
	}

	// ASL: .created_at (inherited from Base) -> base.asl
	loc = SchemaDefinitionIn(modelASL, offsetIn(t, modelASL, "created_at);"), files)
	if loc == nil || loc.URI != baseFile.URI {
		t.Fatalf("created_at loc = %+v, want %s", loc, baseFile.URI)
	}

	// AQL: select Document { id, title, code } -> id in base.asl, title in model.asl
	q := "multi select Document { id, title, code };"
	loc = QueryDefinitionIn(q, offsetIn(t, q, "id,"), schema, files)
	if loc == nil || loc.URI != baseFile.URI {
		t.Fatalf("AQL id loc = %+v, want %s", loc, baseFile.URI)
	}

	loc = QueryDefinitionIn(q, offsetIn(t, q, "title,"), schema, files)
	if loc == nil || loc.URI != modelFile.URI {
		t.Fatalf("AQL title loc = %+v, want %s", loc, modelFile.URI)
	}
}

func lineOf(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return lines[line]
}

