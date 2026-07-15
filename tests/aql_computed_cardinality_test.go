package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/codegen"
)

const cardSchema = `
type GithubInstallation { required id: uuid; required installation_id: int64; }
type Application {
  required id: uuid;
  required name: str;
  link installation: GithubInstallation;
}
type Network { required id: uuid; required link application: Application; required domain: str; }
`

// A plain `select` computed sub-select is a single object: row_to_json over a
// LIMIT 1 inner query, not a json_agg array.
func TestComputedSelectIsSingleObject(t *testing.T) {
	c := compileAQL(t, cardSchema, `select Application {
	  id,
	  installation := (select GithubInstallation { id, installation_id } filter .id = Application.installation)
	} filter .id = $id<uuid>;`)

	if !strings.Contains(c.SQL, "(SELECT row_to_json(g_installation_sub) FROM (") {
		t.Errorf("plain select should compile to a single row_to_json object:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "LIMIT 1) g_installation_sub) AS installation") {
		t.Errorf("single-object sub-select should LIMIT 1:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "json_agg") {
		t.Errorf("plain select must not aggregate into an array:\n%s", c.SQL)
	}
}

// A `multi select` computed sub-select is a JSON array (empty array, not null,
// when nothing matches).
func TestComputedMultiSelectIsArray(t *testing.T) {
	c := compileAQL(t, cardSchema, `select Application {
	  id,
	  networks := (multi select Network filter .application = Application.id)
	} filter .id = $id<uuid>;`)

	if !strings.Contains(c.SQL, "COALESCE(json_agg(row_to_json(n_networks_sub)), '[]')") {
		t.Errorf("multi select should compile to a json_agg array:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "LIMIT 1) n_networks_sub") {
		t.Errorf("multi select must not LIMIT 1:\n%s", c.SQL)
	}
}

// The result descriptor types a plain-select computed field as a single
// (nullable) object and a multi-select one as a repeated field.
func TestComputedCardinalityDescriptor(t *testing.T) {
	ir := parseSchema(t, cardSchema)
	desc := buildQueryDesc(t, ir, "GetApp", "q.aql", `select Application {
	  id,
	  installation := (select GithubInstallation { id } filter .id = Application.installation),
	  networks := (multi select Network filter .application = Application.id)
	} filter .id = $id<uuid>;`)

	byName := map[string]codegen.ResultField{}
	for _, f := range desc.Result.Fields {
		byName[f.Name] = f
	}
	if f := byName["installation"]; f.IsMultiple {
		t.Errorf("plain-select computed field should be single, got IsMultiple=true")
	}
	if f := byName["networks"]; !f.IsMultiple {
		t.Errorf("multi-select computed field should be repeated, got IsMultiple=false")
	}
}

// Projecting a field from a `multi select` is nonsensical (a set, not a row)
// and is rejected.
func TestMultiSelectProjectionRejected(t *testing.T) {
	err := compileErr(t, cardSchema,
		`select Application { id, x := (multi select Network filter .application = Application.id).domain } filter .id = $id<uuid>;`)
	if err == nil || !strings.Contains(err.Error(), "multi select") {
		t.Errorf("expected a multi-select projection error, got %v", err)
	}
}

// `multi` round-trips through the printer.
func TestMultiSubQueryRoundTrips(t *testing.T) {
	src := `select A { xs := (multi select B filter .a = A.id) };`
	stmt, err := aql.ParseString(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := aql.Print(stmt); !strings.Contains(got, "(multi select B") {
		t.Errorf("printed AQL should preserve `multi`:\n%s", got)
	}
}
