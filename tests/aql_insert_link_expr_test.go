package tests

import (
	"strings"
	"testing"
)

const insertLinkSchema = `
type User { required id: uuid; required email: str; }
type Organization {
  required id: uuid;
  required name: str;
  required owner: User;
}
type GithubInstallation {
  required id: uuid;
  required link organization: Organization;
  required installation_id: int64;
}
`

// An insert link assignment accepts a general expression, not just a solo
// subquery: a `??` chain coalescing two lookups resolves the FK from whichever
// side finds a row first.
func TestInsertLinkCoalesceOfSubqueries(t *testing.T) {
	c := compileAQL(t, insertLinkSchema, `
insert GithubInstallation {
  organization := (select Organization filter .id = $org<uuid>?)
               ?? (select GithubInstallation filter .installation_id = $iid<int64>?).organization,
  installation_id := $iid<int64>
};`)

	want := `COALESCE((SELECT o.id FROM "organization" o WHERE ($1::UUID IS NOT NULL AND o.id = $1) LIMIT 1), (SELECT g.organization FROM "github_installation" g WHERE ($2::BIGINT IS NOT NULL AND g.installation_id = $2) LIMIT 1))`
	if !strings.Contains(c.SQL, want) {
		t.Errorf("insert link `??` chain should coalesce the two lookups, want:\n%s\ngot:\n%s", want, c.SQL)
	}
}

// A subquery projection ((select X ...).link) as an insert link assignment
// resolves the projected FK column rather than the row id.
func TestInsertLinkSubqueryProjection(t *testing.T) {
	c := compileAQL(t, insertLinkSchema, `
insert GithubInstallation {
  organization := (select GithubInstallation filter .installation_id = $iid<int64>).organization,
  installation_id := $iid<int64>
};`)

	want := `(SELECT g.organization FROM "github_installation" g WHERE g.installation_id = $1 LIMIT 1)`
	if !strings.Contains(c.SQL, want) {
		t.Errorf("projection should select the organization FK, want:\n%s\ngot:\n%s", want, c.SQL)
	}
}

// In a scalar subquery used as a value, an omitted lone optional param yields no
// row (NULL) rather than matching all rows — otherwise the lookup would return
// an arbitrary organization and a `??` fallback could never fire.
func TestValueSubqueryOptionalParamYieldsNoRow(t *testing.T) {
	c := compileAQL(t, insertLinkSchema, `
insert GithubInstallation {
  organization := (select Organization filter .id = $org<uuid>?),
  installation_id := $iid<int64>
};`)

	if !strings.Contains(c.SQL, `($1::UUID IS NOT NULL AND o.id = $1)`) {
		t.Errorf("value-subquery optional param should use the IS NOT NULL guard:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, `IS NULL OR o.id`) {
		t.Errorf("value-subquery optional param wrongly used the match-all identity:\n%s", c.SQL)
	}
}
