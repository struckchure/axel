package tests

import (
	"regexp"
	"strings"
	"testing"
)

// aliasSchema deliberately chains three tables whose derived aliases collide:
// WorkflowTrigger, Workflow both reduce to "w", so a FILTER traversal across
// them used to reuse the same alias and shadow the base table.
const aliasSchema = `
type Base { required id: uuid; }
type User extends Base { required email: str; }
type Organization extends Base { required owner: User; }
type Workflow extends Base { required organization: Organization; }
type WorkflowTrigger extends Base { required workflow: Workflow; }
`

// A FILTER that traverses a link whose target table shares the base table's
// derived alias must give each table occurrence a distinct alias, and the
// correlated column reference must keep binding to the outer (base) table — not
// rebind to the shadowing inner table. Previously both workflow_trigger and
// workflow aliased "w", so `w.workflow` in the subquery resolved to the inner
// workflow table (a column that does not exist) instead of the base row.
func TestFilterTraversalAliasCollision(t *testing.T) {
	c := compileAQL(t, aliasSchema,
		`select WorkflowTrigger { * } filter .id = $id<uuid> and .workflow.organization.owner = $user<uuid>;`)

	// Base table keeps its bare alias; the inner workflow table gets a distinct
	// one, and the correlation joins the inner workflow row to the base row's FK.
	for _, want := range []string{
		`FROM "workflow_trigger" w`,
		`FROM "workflow" w1`,
		`w1.id = w.workflow`,     // inner workflow correlated to base row
		`o.id = w1.organization`, // organization correlated to inner workflow
		`SELECT o.owner FROM "organization" o`,
	} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("expected SQL to contain %q:\n%s", want, c.SQL)
		}
	}

	// The buggy self-reference where the inner workflow table shadowed the base
	// row must be gone: no `w.id = w.workflow` (a table joined to itself on a
	// non-existent column).
	if strings.Contains(c.SQL, `w.id = w.workflow`) {
		t.Errorf("inner table still shadows the base table (w.id = w.workflow):\n%s", c.SQL)
	}

	// Every emitted table alias must be unique across the whole query.
	assertUniqueAliases(t, c.SQL)
}

// assertUniqueAliases scans `FROM "table" alias` / `JOIN "table" alias`
// occurrences and fails if any alias is reused for two different table
// instances, which is the shadowing condition this fix prevents.
func assertUniqueAliases(t *testing.T, sql string) {
	t.Helper()
	re := regexp.MustCompile(`(?:FROM|JOIN)\s+"[a-z_]+"\s+([a-z][a-z0-9_]*)`)
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(sql, -1) {
		alias := m[1]
		if seen[alias] {
			t.Errorf("alias %q is reused for two table instances:\n%s", alias, sql)
		}
		seen[alias] = true
	}
}

// A membership test against a `multi select` subquery (`x in (multi select Y
// filter ...)`) must NOT cap the subquery at LIMIT 1 — that would turn set
// membership into "equals some arbitrary single row".
func TestInMultiSelectHasNoLimit(t *testing.T) {
	c := compileAQL(t, aliasSchema,
		`multi select Workflow { * } filter .organization in (multi select Organization { id } filter .owner = $user<uuid>);`)

	if !strings.Contains(c.SQL, `in (SELECT o.id FROM "organization" o WHERE o.owner = $1)`) {
		t.Errorf("membership subquery not lowered as expected:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, `$1 LIMIT 1)`) {
		t.Errorf("`in (multi select ...)` must not be capped at LIMIT 1:\n%s", c.SQL)
	}
}

// A scalar (non-multi) subquery still keeps the implicit LIMIT 1 — the multi
// fix must not regress the single-row case.
func TestScalarSubQueryKeepsLimit(t *testing.T) {
	c := compileAQL(t, aliasSchema,
		`select Workflow { * } filter .organization = (select Organization { id } filter .owner = $user<uuid>);`)

	if !strings.Contains(c.SQL, `LIMIT 1)`) {
		t.Errorf("scalar subquery should retain LIMIT 1:\n%s", c.SQL)
	}
}
