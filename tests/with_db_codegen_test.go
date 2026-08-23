package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
	"github.com/struckchure/axel/generators/typescript"
)

func TestWithDbCodegen(t *testing.T) {
	ir := parseSchema(t, `
type Doc { required title: str; }`)
	schema := codegen.FromSchemaIR(ir)
	q := buildQueryDesc(t, ir, "listDocs", "list_docs.aql", `multi select Doc { title };`)

	// TypeScript (bun default)
	tsDir := t.TempDir()
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &typescript.TsGenerator{}, &codegen.Context{OutDir: tsDir}); err != nil {
		t.Fatalf("ts walk: %v", err)
	}
	tsRunner := readFile(t, filepath.Join(tsDir, "runner.ts"))
	for _, want := range []string{
		"withDb(db: DB): Queries;",
		"withDb<T>(db: DB, fn: (q: Queries) => Promise<T>): Promise<T>;",
		"withDb<T>(db: DB, fn?: (q: Queries) => Promise<T>): Queries | Promise<T>",
	} {
		if strings.Count(tsRunner, want) < 2 { // present in both Queries and Runner
			t.Errorf("ts runner.ts expected 2 occurrences of %q, got %d:\n%s", want, strings.Count(tsRunner, want), tsRunner)
		}
	}

	// TypeScript (pg client)
	tsPgDir := t.TempDir()
	tsPgCtx := &codegen.Context{OutDir: tsPgDir, Options: map[string]string{"client": "pg"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &typescript.TsGenerator{}, tsPgCtx); err != nil {
		t.Fatalf("ts pg walk: %v", err)
	}
	tsPgRunner := readFile(t, filepath.Join(tsPgDir, "runner.ts"))
	for _, want := range []string{
		"withDb(db: Pool): Queries;",
		"withDb<T>(db: Pool, fn: (q: Queries) => Promise<T>): Promise<T>;",
		"withDb<T>(db: Pool, fn?: (q: Queries) => Promise<T>): Queries | Promise<T>",
	} {
		if strings.Count(tsPgRunner, want) < 2 {
			t.Errorf("ts pg runner.ts expected 2 occurrences of %q, got %d:\n%s", want, strings.Count(tsPgRunner, want), tsPgRunner)
		}
	}

	// Go
	goDir := t.TempDir()
	goCtx := &codegen.Context{OutDir: goDir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &golang.GoGenerator{}, goCtx); err != nil {
		t.Fatalf("go walk: %v", err)
	}
	goRunner := readFile(t, filepath.Join(goDir, "runner.go"))
	for _, want := range []string{
		"func NewQueries(db DBTX) *Queries",
		"func (q *Queries) WithDB(db DBTX) *Queries",
		"func (r *Runner) WithDB(db DBTX) *Queries",
	} {
		if !strings.Contains(goRunner, want) {
			t.Errorf("go runner.go missing %q:\n%s", want, goRunner)
		}
	}
}
