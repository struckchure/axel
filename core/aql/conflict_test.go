package aql

import (
	"strings"
	"testing"
)

// The `unless conflict` clause parses in all three forms and round-trips through
// the printer.
func TestParseConflictForms(t *testing.T) {
	cases := []struct {
		name  string
		query string
		check func(t *testing.T, c *OnConflict)
	}{
		{
			name:  "bare",
			query: `insert User { email := $email } unless conflict;`,
			check: func(t *testing.T, c *OnConflict) {
				if c.Target != nil || c.Else != nil {
					t.Errorf("bare conflict should have no target or else: %+v", c)
				}
			},
		},
		{
			name:  "on single",
			query: `insert User { email := $email } unless conflict on .email;`,
			check: func(t *testing.T, c *OnConflict) {
				if c.Target == nil || len(c.Target.Fields) != 1 || c.Target.Fields[0] != "email" {
					t.Errorf("expected target [email], got %+v", c.Target)
				}
				if c.Else != nil {
					t.Errorf("expected no else arm")
				}
			},
		},
		{
			name:  "on composite",
			query: `insert User { email := $email } unless conflict on (.email, .tenant_id);`,
			check: func(t *testing.T, c *OnConflict) {
				if c.Target == nil || len(c.Target.Fields) != 2 {
					t.Fatalf("expected 2-field target, got %+v", c.Target)
				}
				if c.Target.Fields[0] != "email" || c.Target.Fields[1] != "tenant_id" {
					t.Errorf("expected [email tenant_id], got %v", c.Target.Fields)
				}
			},
		},
		{
			name:  "else update",
			query: `insert User { email := $email, name := $name } unless conflict on .email else (update User set { name := $name });`,
			check: func(t *testing.T, c *OnConflict) {
				if c.Else == nil {
					t.Fatalf("expected else arm")
				}
				if c.Else.TypeName != "User" {
					t.Errorf("expected else type User, got %q", c.Else.TypeName)
				}
				if len(c.Else.Assignments) != 1 || c.Else.Assignments[0].Field != "name" {
					t.Errorf("expected else set { name := ... }, got %+v", c.Else.Assignments)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt, err := ParseString(tc.query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if stmt.Insert == nil || stmt.Insert.Conflict == nil {
				t.Fatalf("expected insert with conflict clause")
			}
			tc.check(t, stmt.Insert.Conflict)

			// Round-trip: printing then re-parsing must preserve the clause.
			printed := Print(stmt)
			if !strings.Contains(printed, "unless conflict") {
				t.Errorf("printed output missing conflict clause:\n%s", printed)
			}
			if _, err := ParseString(printed); err != nil {
				t.Errorf("re-parse of printed output failed: %v\n%s", err, printed)
			}
		})
	}
}
