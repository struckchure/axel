package tests

import (
	"strings"
	"testing"

	axel "github.com/struckchure/axel/core"
)

func TestIndexesEmittedInCreateTable(t *testing.T) {
	up := genUp(t, `
type User {
  required id: uuid { constraint pk; };
  required email: str;
  required age: int32;
  active: bool { default := true };

  index on (.email);
  index on (.active, .age);
}
`)
	for _, want := range []string{
		`CREATE INDEX IF NOT EXISTS "idx_user_email" ON "user" ("email");`,
		`CREATE INDEX IF NOT EXISTS "idx_user_active_age" ON "user" ("active", "age");`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("up SQL missing %q:\n%s", want, up)
		}
	}
}

func TestAddIndexToExistingTable(t *testing.T) {
	base := `type User { required id: uuid { constraint pk; }; required email: str; }`
	withIndex := `type User { required id: uuid { constraint pk; }; required email: str; index on (.email); }`

	up, down := genMigration(t, base, withIndex)

	if !strings.Contains(up, `CREATE INDEX IF NOT EXISTS "idx_user_email" ON "user" ("email");`) {
		t.Errorf("up SQL missing CREATE INDEX:\n%s", up)
	}
	if !strings.Contains(down, `DROP INDEX IF EXISTS "idx_user_email";`) {
		t.Errorf("down SQL missing DROP INDEX:\n%s", down)
	}
}

func TestDiffProducesIndexChangeTypes(t *testing.T) {
	oldModels := parseToModels(t, `type User { required id: uuid { constraint pk; }; required email: str; index on (.email); }`)
	newModels := parseToModels(t, `type User { required id: uuid { constraint pk; }; required email: str; required age: int32; index on (.age); }`)

	changes := axel.DiffSchemas(oldModels, newModels)

	var added, dropped bool
	for _, c := range changes {
		switch c.Type {
		case axel.AddIndex:
			if idx, ok := c.NewValue.(axel.Index); ok && strings.Join(idx.Columns, ",") == "age" {
				added = true
			}
		case axel.DropIndex:
			if idx, ok := c.OldValue.(axel.Index); ok && strings.Join(idx.Columns, ",") == "email" {
				dropped = true
			}
		}
	}
	if !added || !dropped {
		t.Errorf("expected AddIndex(age) and DropIndex(email), got added=%v dropped=%v", added, dropped)
	}
}

func TestAddIndexChangeSQL(t *testing.T) {
	changes := []axel.SchemaChange{
		{Type: axel.AddIndex, ModelName: "User", NewValue: axel.Index{Columns: []string{"email"}}},
	}
	up, down := axel.GenerateMigrationSQL(changes, nil, nil)

	if !strings.Contains(up, `CREATE INDEX IF NOT EXISTS "idx_user_email" ON "user" ("email");`) {
		t.Errorf("up SQL missing CREATE INDEX:\n%s", up)
	}
	if !strings.Contains(down, `DROP INDEX IF EXISTS "idx_user_email";`) {
		t.Errorf("down SQL missing DROP INDEX:\n%s", down)
	}
}
