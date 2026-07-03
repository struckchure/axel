package axel

import (
	"strings"
	"testing"
)

func TestGenerateColumnLengthChecks(t *testing.T) {
	field := Field{
		Name: "email",
		Type: "str",
		Constraints: []Constraint{
			{Name: "exclusive"},
			{Name: "min_length", Args: []string{"6"}},
			{Name: "max_length", Args: []string{"100"}},
		},
	}

	col, _ := generateColumn(field, "User")

	for _, want := range []string{
		"UNIQUE",
		`CHECK (char_length("email") >= 6)`,
		`CHECK (char_length("email") <= 100)`,
	} {
		if !strings.Contains(col, want) {
			t.Errorf("column %q missing %q", col, want)
		}
	}
}

func TestLengthChecksSkippedForNonString(t *testing.T) {
	field := Field{
		Name:        "age",
		Type:        "int32",
		Constraints: []Constraint{{Name: "min_length", Args: []string{"6"}}},
	}

	if got := lengthCheckClauses(`"age"`, field); got != nil {
		t.Errorf("expected no length checks for int32, got %v", got)
	}
}

func TestGenerateIndexes(t *testing.T) {
	model := Model{
		Name: "User",
		Indexes: []Index{
			{Columns: []string{"email"}},
			{Columns: []string{"active", "age"}},
		},
	}

	got := generateIndexes(model)
	want := []string{
		`CREATE INDEX IF NOT EXISTS "idx_user_email" ON "user" ("email");`,
		`CREATE INDEX IF NOT EXISTS "idx_user_active_age" ON "user" ("active", "age");`,
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d index statements, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGenerateTableEmitsIndexes(t *testing.T) {
	model := Model{
		Name: "User",
		Fields: []Field{
			{Name: "email", Type: "str", IsRequired: true},
		},
		Indexes: []Index{{Columns: []string{"email"}}},
	}

	sql := generateTable(model, map[string]Model{})
	if !strings.Contains(sql, `CREATE INDEX IF NOT EXISTS "idx_user_email"`) {
		t.Errorf("generateTable output missing CREATE INDEX:\n%s", sql)
	}
}

func TestDiffIndexesAddAndDrop(t *testing.T) {
	oldModel := Model{Name: "User", Indexes: []Index{{Columns: []string{"email"}}}}
	newModel := Model{Name: "User", Indexes: []Index{{Columns: []string{"active", "age"}}}}

	changes := diffIndexes(oldModel, newModel)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(changes), changes)
	}

	var added, dropped bool
	for _, c := range changes {
		switch c.Type {
		case AddIndex:
			added = true
			if idx, ok := c.NewValue.(Index); !ok || strings.Join(idx.Columns, ",") != "active,age" {
				t.Errorf("unexpected AddIndex value: %+v", c.NewValue)
			}
		case DropIndex:
			dropped = true
			if idx, ok := c.OldValue.(Index); !ok || strings.Join(idx.Columns, ",") != "email" {
				t.Errorf("unexpected DropIndex value: %+v", c.OldValue)
			}
		}
	}
	if !added || !dropped {
		t.Errorf("expected one add and one drop, got added=%v dropped=%v", added, dropped)
	}
}

func TestGenerateMigrationSQLIndexChange(t *testing.T) {
	changes := []SchemaChange{
		{Type: AddIndex, ModelName: "User", NewValue: Index{Columns: []string{"email"}}},
	}

	up, down := GenerateMigrationSQL(changes, nil, nil)

	if !strings.Contains(up, `CREATE INDEX IF NOT EXISTS "idx_user_email" ON "user" ("email");`) {
		t.Errorf("up SQL missing CREATE INDEX:\n%s", up)
	}
	if !strings.Contains(down, `DROP INDEX IF EXISTS "idx_user_email";`) {
		t.Errorf("down SQL missing DROP INDEX:\n%s", down)
	}
}

func TestGenerateModifyColumnLengthConstraint(t *testing.T) {
	oldField := Field{Name: "email", Type: "str"}
	newField := Field{Name: "email", Type: "str", Constraints: []Constraint{{Name: "min_length", Args: []string{"6"}}}}

	up, down := generateModifyColumn("user", oldField, newField)

	wantUp := `ALTER TABLE "user" ADD CONSTRAINT chk_user_email_min_length CHECK (char_length("email") >= 6);`
	if !strings.Contains(up, wantUp) {
		t.Errorf("up SQL missing add constraint:\n%s", up)
	}
	wantDown := `ALTER TABLE "user" DROP CONSTRAINT IF EXISTS chk_user_email_min_length;`
	if !strings.Contains(down, wantDown) {
		t.Errorf("down SQL missing drop constraint:\n%s", down)
	}
}
