package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/codegen"
)

const pathSchema = `
type User { required id: uuid; required email: str; }
type Organization { required id: uuid; required owner: User; required name: str; }
type Project { required id: uuid; required organization: Organization; required owner: User; }
type GithubInstallation { required id: uuid; required installation_id: int64; }
type Application {
  required id: uuid;
  required name: str;
  project: Project;
  installation: GithubInstallation;
}
`

// A multi-hop path in a computed field resolves through the links, and a `<Type>`
// cast wraps the result in ::SQLTYPE.
func TestPathCastInComputedField(t *testing.T) {
	c := compileAQL(t, pathSchema, `multi select Application {
	  id,
	  owner := .project.organization.owner.id<uuid>
	} filter .id = $id<uuid>;`)

	if !strings.Contains(c.SQL, ")::UUID) AS owner") {
		t.Errorf("path cast should wrap the resolved path in ::UUID:\n%s", c.SQL)
	}
	// The multi-hop traversal itself still resolves through the links.
	if !strings.Contains(c.SQL, `FROM "organization" o WHERE o.id = p.organization`) {
		t.Errorf("multi-hop path should traverse project → organization:\n%s", c.SQL)
	}
}

// The result descriptor types a cast path field by the cast, not json.
func TestPathCastTypesResultField(t *testing.T) {
	ir := parseSchema(t, pathSchema)
	desc := buildQueryDesc(t, ir, "Q", "q.aql", `multi select Application {
	  id,
	  owner := .project.organization.owner.id<uuid>
	} filter .id = $id<uuid>;`)

	var owner *codegen.ResultField
	for i := range desc.Result.Fields {
		if desc.Result.Fields[i].Name == "owner" {
			owner = &desc.Result.Fields[i]
		}
	}
	if owner == nil {
		t.Fatalf("owner field missing from descriptor")
	}
	if owner.AQLType != "uuid" {
		t.Errorf("cast path field should be typed uuid, got %q (SQL %q)", owner.AQLType, owner.SQLType)
	}
}

// A cast also applies to a single-step path, and works outside computed fields
// (e.g. in a filter).
func TestPathCastInFilter(t *testing.T) {
	c := compileAQL(t, pathSchema, `multi select Organization { id } filter .name<str> = $n<str>;`)
	if !strings.Contains(c.SQL, "(o.name)::TEXT = $1") {
		t.Errorf("path cast should apply in a filter:\n%s", c.SQL)
	}
}

// An enum cast resolves to TEXT; an unknown cast type is an error.
func TestPathCastEnumAndUnknown(t *testing.T) {
	enumSchema := `
enum Status { Active, Archived }
type Org { required id: uuid; status: Status; }`
	c := compileAQL(t, enumSchema, `multi select Org { id, s := .status<Status> };`)
	if !strings.Contains(c.SQL, "(o.status)::TEXT) AS s") {
		t.Errorf("enum path cast should resolve to ::TEXT:\n%s", c.SQL)
	}

	if err := compileErr(t, pathSchema, `multi select Organization { id, x := .name<Nope> };`); err == nil || !strings.Contains(err.Error(), "Nope") {
		t.Errorf("expected an unknown-cast-type error, got %v", err)
	}
}

// The cast round-trips through the printer, and a `<` comparison is unaffected.
func TestPathCastRoundTripsAndComparisonUnaffected(t *testing.T) {
	stmt, err := aql.ParseString(`select A { o := .b.c<uuid> };`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := aql.Print(stmt); !strings.Contains(got, ".b.c<uuid>") {
		t.Errorf("printed AQL should preserve the path cast:\n%s", got)
	}

	// `.id < $x` is still a comparison, not a cast.
	c := compileAQL(t, pathSchema, `multi select Organization { id } filter .id < $max<uuid>;`)
	if !strings.Contains(c.SQL, "o.id < $1") {
		t.Errorf("`.id < $x` should stay a comparison:\n%s", c.SQL)
	}
}
