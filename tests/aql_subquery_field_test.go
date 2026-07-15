package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
)

const subqSchema = `
type GithubInstallation {
  required id: uuid;
  required installation_id: str;
  required account_login: str;
}
type Repo {
  required id: uuid;
  required name: str;
  required installation_id: str;
  required link owner: GithubInstallation;
}
`

// A subquery may project a named field instead of its id:
// (select T filter ...).field  →  SELECT t.<column> ... LIMIT 1
func TestSubQueryProjectsField(t *testing.T) {
	c := compileAQL(t, subqSchema,
		`multi select Repo { id, x := (select GithubInstallation filter .id = $gid<uuid>).installation_id };`)

	if !strings.Contains(c.SQL, `SELECT g.installation_id FROM "github_installation" g WHERE g.id = $1 LIMIT 1`) {
		t.Errorf("subquery should project installation_id:\n%s", c.SQL)
	}
	// A projected subquery is a scalar, not a json_agg of the row.
	if strings.Contains(c.SQL, "json_agg") {
		t.Errorf("projected subquery must not aggregate the whole row:\n%s", c.SQL)
	}
}

// The motivating case: an assignment that reads a field from a subquery and
// coalesces it, i.e. (select ...).field ?? .field.
func TestSubQueryFieldInCoalesceAssignment(t *testing.T) {
	c := compileAQL(t, subqSchema, `update Repo filter .id = $id set {
	  installation_id := (select GithubInstallation filter .id = $gid<uuid>?).installation_id ?? .installation_id
	};`)

	want := `installation_id = COALESCE((SELECT g.installation_id FROM "github_installation" g WHERE ($1::UUID IS NULL OR g.id = $1) LIMIT 1), r.installation_id)`
	if !strings.Contains(c.SQL, want) {
		t.Errorf("expected coalesce of projected subquery with current value, want:\n%s\ngot:\n%s", want, c.SQL)
	}
}

// A link may be projected too — it resolves to the FK join column.
func TestSubQueryProjectsLinkColumn(t *testing.T) {
	c := compileAQL(t, subqSchema,
		`multi select Repo { id, o := (select Repo filter .id = $rid<uuid>).owner };`)
	if !strings.Contains(c.SQL, `SELECT r.owner FROM "repo" r WHERE r.id = $1 LIMIT 1`) {
		t.Errorf("subquery should project the owner FK column:\n%s", c.SQL)
	}
}

// Projecting a field the type does not have is a compile error.
func TestSubQueryProjectUnknownFieldErrors(t *testing.T) {
	err := compileErr(t, subqSchema, `multi select Repo { id, x := (select GithubInstallation filter .id = $gid<uuid>).nope };`)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("expected an unknown-field error for the projection, got %v", err)
	}
}

// The projection round-trips through the printer.
func TestSubQueryProjectionRoundTrips(t *testing.T) {
	src := `select Repo { id, x := (select GithubInstallation filter .id = $gid).installation_id };`
	stmt, err := aql.ParseString(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := aql.Print(stmt); !strings.Contains(got, "(select GithubInstallation filter .id = $gid).installation_id") {
		t.Errorf("printed AQL should preserve the projection:\n%s", got)
	}
}
