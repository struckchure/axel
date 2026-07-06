package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
)

const linkSchema = `
type User { required id: uuid; required email: str; }
type Application { required id: uuid; required name: str; }
type Membership {
  required id: uuid;
  required link user: User;
  link application: Application;
}
`

// A shapeless `select *` on a type with relationships must also select the
// single-link FK columns, so reference fields appear (and can be scanned) in
// the generated row type.
func TestSelectStarIncludesLinkReferenceFields(t *testing.T) {
	c := compileAQL(t, linkSchema, `multi select Membership;`)
	for _, want := range []string{"m.application", "m.user"} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("shapeless select should select FK column %q:\n%s", want, c.SQL)
		}
	}

	ir := parseSchema(t, linkSchema)
	schema := codegen.FromSchemaIR(ir)
	q := buildQueryDesc(t, ir, "", "list.aql", "@response MembershipRow\nmulti select Membership;")

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &golang.GoGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	models := readFile(t, filepath.Join(dir, "models.go"))
	// The row's FK fields carry matching json/db tags naming the actual column.
	for _, want := range []string{`json:"application" db:"application"`, `json:"user" db:"user"`} {
		if !strings.Contains(models, want) {
			t.Errorf("MembershipRow should include reference field tag %q:\n%s", want, models)
		}
	}
}
