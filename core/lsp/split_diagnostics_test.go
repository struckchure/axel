package lsp

import (
	"strings"
	"testing"
)

func TestSchemaDiagnosticsInMergesSiblings(t *testing.T) {
	book := "type Book {\n  required title: str;\n  required link author: Author;\n}\n"
	author := "type Author {\n  required name: str;\n}\n"

	// On its own, Book's link target is unknown.
	if got := SchemaDiagnostics(book); len(got) == 0 {
		t.Fatal("expected an unknown-type diagnostic for the lone file")
	}

	// Merged with its sibling, it is clean.
	others := []SchemaFile{{Path: "author.asl", Text: author}}
	if got := SchemaDiagnosticsIn("book.asl", book, others); len(got) != 0 {
		t.Errorf("expected no diagnostics, got %+v", got)
	}
}

func TestSchemaDiagnosticsInReportsOnlyLocalProblems(t *testing.T) {
	// The broken link lives in the sibling; this file is fine and should stay quiet.
	ok := "type Author {\n  required name: str;\n}\n"
	broken := "type Book {\n  required link author: Nope;\n}\n"

	if got := SchemaDiagnosticsIn("author.asl", ok, []SchemaFile{{Path: "book.asl", Text: broken}}); len(got) != 0 {
		t.Errorf("sibling's problem reported here: %+v", got)
	}
	got := SchemaDiagnosticsIn("book.asl", broken, []SchemaFile{{Path: "author.asl", Text: ok}})
	if len(got) != 1 {
		t.Fatalf("want 1 diagnostic on the owning file, got %+v", got)
	}
}

func TestSchemaDiagnosticsInReportsRedeclaration(t *testing.T) {
	a := "type User {\n  required name: str;\n}\n"
	b := "type User {\n  required email: str;\n}\n"

	got := SchemaDiagnosticsIn("a.asl", a, []SchemaFile{{Path: "b.asl", Text: b}})
	if len(got) != 1 {
		t.Fatalf("want 1 diagnostic, got %+v", got)
	}
	if !strings.Contains(got[0].Message, "b.asl") {
		t.Errorf("message should name the other file: %q", got[0].Message)
	}
}

// A sibling that is mid-edit and does not parse must not blank this file's
// diagnostics or crash the pass.
func TestSchemaDiagnosticsInToleratesUnparseableSibling(t *testing.T) {
	good := "type Author {\n  required name: str;\n}\n"
	garbage := "type Book { required"

	got := SchemaDiagnosticsIn("author.asl", good, []SchemaFile{{Path: "book.asl", Text: garbage}})
	if len(got) != 0 {
		t.Errorf("expected no diagnostics, got %+v", got)
	}
}

func TestQueryDiagnosticsFunctionParams(t *testing.T) {
	schema := schemaFor(t, `
function random_string(len: int32) -> str { return 'abc'; };
type User { required name: str; }
`)

	// Valid call: 1 argument
	okQuery := `select User filter random_string(10) != '';`
	if diags := QueryDiagnostics(okQuery, schema); len(diags) != 0 {
		t.Fatalf("expected no diagnostics for valid function call, got %+v", diags)
	}

	// Invalid call: 2 arguments
	badQuery := `select User filter random_string(10, 20) != '';`
	diags := QueryDiagnostics(badQuery, schema)
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for invalid function argument count, got none")
	}
	if !strings.Contains(diags[0].Message, `function "random_string" expects 1 argument(s), got 2`) {
		t.Errorf("unexpected diagnostic message: %q", diags[0].Message)
	}

	// Invalid call: argument type mismatch ('10' passed to int32)
	badTypeQuery := `select User filter random_string('10') != '';`
	diags = QueryDiagnostics(badTypeQuery, schema)
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for function argument type mismatch, got none")
	}
	if !strings.Contains(diags[0].Message, `function "random_string" argument 1 expects int32, got str`) {
		t.Errorf("unexpected diagnostic message: %q", diags[0].Message)
	}
}

func TestSchemaDiagnosticsFunctionParamTypeMismatch(t *testing.T) {
	aslText := `
function random_hex(len: int32) -> str { return 'hex'; };

scalar type Code extends str {
  default := random_hex('6');
}
`
	diags := SchemaDiagnostics(aslText)
	if len(diags) == 0 {
		t.Fatal("expected schema diagnostic for default function argument type mismatch, got none")
	}
	if !strings.Contains(diags[0].Message, `function "random_hex" argument 1 expects int32, got str ('6')`) {
		t.Errorf("unexpected diagnostic message: %q", diags[0].Message)
	}
}


