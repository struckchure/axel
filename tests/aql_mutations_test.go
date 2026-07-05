package tests

import (
	"strings"
	"testing"
)

// Mutations must compile to a SINGLE SQL command — no embedded BEGIN;/COMMIT; —
// otherwise the parameterized (extended) protocol rejects them with
// "cannot insert multiple commands into a prepared statement" (42601).
func TestMutationsAreSingleStatement(t *testing.T) {
	schema := `type User { required id: uuid; required email: str; name: str; }`

	cases := map[string]string{
		"insert": `insert User { email := $email, name := $name };`,
		"update": `update User filter .id = $id set { name := $name };`,
		"delete": `delete User filter .id = $id;`,
	}

	for op, q := range cases {
		c := compileAQL(t, schema, q)
		if strings.Contains(c.SQL, "BEGIN") || strings.Contains(c.SQL, "COMMIT") {
			t.Errorf("%s SQL should not wrap BEGIN/COMMIT:\n%s", op, c.SQL)
		}
		// A single trailing ';' → exactly one command.
		if n := strings.Count(strings.TrimRight(c.SQL, "\n"), ";"); n != 1 {
			t.Errorf("%s SQL should be a single statement (found %d ';'):\n%s", op, n, c.SQL)
		}
	}
}
