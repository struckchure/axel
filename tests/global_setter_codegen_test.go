package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
	"github.com/struckchure/axel/generators/typescript"
)

// A declared global must produce, in both the TypeScript and Go clients:
//   - a transaction-scoped Runner setter (with<Name>), and
//   - a functional-options / options-object path on the standalone query
//     functions, so a global can be set without the Runner.
//
// Both push the value via set_config('app.<name>', …).
func TestGlobalSetterCodegen(t *testing.T) {
	ir := parseSchema(t, `
global current_user: uuid;
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
		"async withCurrentUser<T>(value: string,",  // Runner setter
		"set_config('app.current_user', $1, true)", // set via set_config
		"begin<T>(fn: (tx: DB) => Promise<T>)",     // DB gains begin
		"export interface GlobalOptions {",         // options object
		"currentUser?: string;",
		"export async function _withGlobals<T>", // free-function wrapper
	} {
		if !strings.Contains(tsRunner, want) {
			t.Errorf("ts runner.ts missing %q:\n%s", want, tsRunner)
		}
	}
	tsQuery := readFile(t, filepath.Join(tsDir, "list_docs.ts"))
	if !strings.Contains(tsQuery, "opts?: GlobalOptions") {
		t.Errorf("ts query fn missing opts param:\n%s", tsQuery)
	}

	// Go
	goDir := t.TempDir()
	goCtx := &codegen.Context{OutDir: goDir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &golang.GoGenerator{}, goCtx); err != nil {
		t.Fatalf("go walk: %v", err)
	}
	goRunner := readFile(t, filepath.Join(goDir, "runner.go"))
	for _, want := range []string{
		"func (r *Runner) WithCurrentUser(ctx context.Context, value string, fn func(*Queries) error) error", // Runner setter
		"set_config('app.current_user', $1, true)",
		"type DBTX interface",
		"type Option func(*globalOpts)",             // functional options
		"func WithCurrentUser(value string) Option", // option constructor
		"func beginWithGlobals(",
	} {
		if !strings.Contains(goRunner, want) {
			t.Errorf("go runner.go missing %q:\n%s", want, goRunner)
		}
	}
	goQuery := readFile(t, filepath.Join(goDir, "list_docs.go"))
	if !strings.Contains(goQuery, "opts ...Option") {
		t.Errorf("go query fn missing opts param:\n%s", goQuery)
	}
}
