package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
)

// Abstract types have no table and are never queried directly; the Go generator
// must not emit a model struct for them (their fields are inlined into the
// concrete types that extend them).
func TestGoCodegenSkipsAbstractTypes(t *testing.T) {
	ir := parseSchema(t, `
abstract type Base { required id: uuid; }
type User extending Base { required email: str; }
`)
	schema := codegen.FromSchemaIR(ir)

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, nil, &golang.GoGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	models := readFile(t, filepath.Join(dir, "models.go"))
	// Collapse gofmt's alignment padding so assertions are whitespace-insensitive.
	collapsed := strings.Join(strings.Fields(models), " ")
	if strings.Contains(collapsed, "type Base struct") {
		t.Errorf("abstract type Base should not be emitted as a struct:\n%s", models)
	}
	if !strings.Contains(collapsed, "type User struct") {
		t.Errorf("concrete type User should still be emitted:\n%s", models)
	}
	// User must carry the inherited field from Base.
	if !strings.Contains(collapsed, "ID string") {
		t.Errorf("User should inline the inherited id field:\n%s", models)
	}
}
