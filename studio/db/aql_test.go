package db

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/samber/lo"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/compiler"
)

const schemaFixture = "../../examples/basic/default.asl"

func loadFixture(t *testing.T) *Schema {
	t.Helper()
	s, err := LoadSchema(schemaFixture)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	return s
}

// compiles asserts an AQL string parses and lowers to SQL against the schema.
func compiles(t *testing.T, s *Schema, query string) {
	t.Helper()
	stmt, err := aql.ParseString(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	if _, err := compiler.Compile(stmt, s.IR); err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
}

func TestSchemaTablesExcludeAbstract(t *testing.T) {
	s := loadFixture(t)
	tables := s.Tables()
	if len(tables) == 0 {
		t.Fatal("no tables from schema")
	}
	for _, tb := range tables {
		if tb.Type == "Base" {
			t.Errorf("abstract type Base must not appear as a table")
		}
		if tb.Type == "" || tb.Name == "" {
			t.Errorf("table missing type/name: %+v", tb)
		}
	}
}

// Every generated read query (and its count) must be valid AQL that compiles.
func TestGeneratedReadsCompile(t *testing.T) {
	s := loadFixture(t)
	for _, tb := range s.Tables() {
		cols := gridColumns(tb)
		if len(cols) == 0 {
			t.Errorf("%s: no grid columns", tb.Type)
		}
		read := buildReadAQL(tb.Type, cols, 50, 0, nil)
		compiles(t, s, read)

		sorted := buildReadAQL(tb.Type, cols, 50, 0, &Order{Column: "id", Desc: true})
		compiles(t, s, sorted)

		compiles(t, s, "select count("+tb.Type+");")
	}
}

func TestExecCompileOnly(t *testing.T) {
	s := loadFixture(t)
	engine := NewAQL(s, nil) // no pool → compile-only
	if engine.Live() {
		t.Fatal("engine with nil pool should not be live")
	}

	got := engine.Exec(context.Background(), "multi select User { id, email } filter .active = true;")
	if got.Err != "" {
		t.Fatalf("unexpected error: %s", got.Err)
	}
	if got.SQL == "" || !strings.Contains(strings.ToUpper(got.SQL), "SELECT") {
		t.Fatalf("expected compiled SQL, got %q", got.SQL)
	}
}

func TestExecReportsParseErrors(t *testing.T) {
	s := loadFixture(t)
	engine := NewAQL(s, nil)
	got := engine.Exec(context.Background(), "this is not aql")
	if got.Err == "" {
		t.Fatal("expected a parse/compile error")
	}
}

func TestWriteAQLCompiles(t *testing.T) {
	s := loadFixture(t)
	compiles(t, s, "insert User { email := $email, age := 100, health := 100 };")
	compiles(t, s, "update User filter .id = $id set { health := 50 };")
	compiles(t, s, "delete User filter .id = $id;")
}

func TestFlattenLink(t *testing.T) {
	single := map[string]any{"id": "u-1"}
	if got := flattenLink(single, Column{IsLink: true}); got != "u-1" {
		t.Errorf("single link: got %v, want u-1", got)
	}
	multi := []any{map[string]any{"id": "u-1"}, map[string]any{"id": "u-2"}}
	got := flattenLink(multi, Column{IsLink: true, IsMulti: true})
	ids, ok := got.([]any)
	if !ok || len(ids) != 2 || ids[0] != "u-1" || ids[1] != "u-2" {
		t.Errorf("multi link: got %v, want [u-1 u-2]", got)
	}
	if got := flattenLink(nil, Column{IsLink: true}); got != nil {
		t.Errorf("nil single link: got %v, want nil", got)
	}
}

// Post has both a single link (author) and a multi link (likes); its generated
// read must select and compile both.
func TestReadIncludesLinks(t *testing.T) {
	s := loadFixture(t)
	post, ok := s.Table("post")
	if !ok {
		t.Fatal("post table not found")
	}
	var single, multi bool
	for _, c := range gridColumns(post) {
		if c.Name == "author" && c.IsLink && !c.IsMulti {
			single = true
		}
		if c.Name == "likes" && c.IsLink && c.IsMulti {
			multi = true
		}
	}
	if !single || !multi {
		t.Fatalf("expected single+multi links in grid columns (single=%v multi=%v)", single, multi)
	}
	compiles(t, s, buildReadAQL("Post", gridColumns(post), 50, 0, nil))
}

func TestAssignmentsLinks(t *testing.T) {
	s := loadFixture(t)
	a := NewAQL(s, nil) // compile-only

	// Single link resolves to a sub-select; its value binds as a param.
	assigns, params, err := a.assignments("Post",
		map[string]string{"title": "T", "content": "C", "author": "ID1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(assigns, ", ")
	if !strings.Contains(joined, "author := (select User filter .id = $author)") {
		t.Errorf("expected single-link sub-select, got: %s", joined)
	}
	if params["author"] != "ID1" {
		t.Errorf("author param = %v, want ID1", params["author"])
	}
	compiles(t, s, fmt.Sprintf("insert Post { %s };", joined))

	// A multi link never appears in Set (it is reconciled via the junction
	// table), so passing one through assignments must not emit AQL for it.
	assigns2, _, err := a.assignments("Post",
		map[string]string{"title": "T", "content": "C", "likes": "X"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(assigns2, ", "), "likes") {
		t.Errorf("multi link should be skipped, got: %v", assigns2)
	}
}

// TestJunctionNaming locks in the junction table/column naming convention.
func TestJunctionNaming(t *testing.T) {
	if got := lo.SnakeCase("Post") + "_" + lo.SnakeCase("likes"); got != "post_likes" {
		t.Errorf("junction table = %q, want post_likes", got)
	}
	if got := lo.SnakeCase("User"); got != "user" {
		t.Errorf("target column = %q, want user", got)
	}
}
