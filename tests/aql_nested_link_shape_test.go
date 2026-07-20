package tests

import (
	"strings"
	"testing"
)

const nestedLinkSchema = `
type User { required id: uuid; required email: str; }
type Organization {
  required id: uuid;
  required name: str;
  required owner: User;
}
type Project {
  required id: uuid;
  required organization: Organization;
  required name: str;
}
type Application {
  required id: uuid;
  link project: Project;
  required name: str;
}
`

// A link sub-shape may itself select nested links, not just scalar properties.
// `project: { id, organization: { id } }` must compile the inner `organization`
// link into a correlated json subquery nested inside the project subquery.
func TestNestedLinkSubShape(t *testing.T) {
	c := compileAQL(t, nestedLinkSchema, `
select Application {
  id,
  project: {
    id,
    organization: {
      id
    }
  }
};`)

	// The project subquery projects an `organization` column whose value is
	// itself a row_to_json subquery over the organization table.
	for _, want := range []string{
		`AS project`,
		`AS organization`,
		`FROM "organization"`,
		`FROM "project"`,
	} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("expected SQL to contain %q:\n%s", want, c.SQL)
		}
	}
}

// A multi-step path that ends in a link's implicit id resolves to the FK column
// through the intervening link, so `.project.organization.id` filters against
// project's organization FK without error.
func TestNestedLinkPathFilter(t *testing.T) {
	c := compileAQL(t, nestedLinkSchema, `
multi select Application {
  *,
  project: { id, organization: { id } }
}
filter
  .project.organization.owner = $user<uuid>
  and .project.organization.id = $organization<uuid>?;`)

	if !strings.Contains(c.SQL, `WHERE`) {
		t.Fatalf("expected a WHERE clause:\n%s", c.SQL)
	}
	// The `.project.organization.owner` path nests correlated subqueries.
	if !strings.Contains(c.SQL, `FROM "organization"`) || !strings.Contains(c.SQL, `FROM "project"`) {
		t.Errorf("expected nested link subqueries in filter:\n%s", c.SQL)
	}
}
