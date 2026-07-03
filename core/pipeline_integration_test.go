package axel

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/asl"
)

// TestPipelineEmitsConstraintsAndIndexes exercises the full
// ASL → SchemaIR → []Model → SQL path to confirm length constraints and
// indexes reach the generated migration SQL.
func TestPipelineEmitsConstraintsAndIndexes(t *testing.T) {
	schema := `
type User {
  required id: uuid { constraint pk; };
  required email: str {
    constraint exclusive;
    constraint min_length(6);
    constraint max_length(100);
  };
  active: bool { default := true };
  required age: int32;

  index on (.email);
  index on (.active, .age);
}
`

	src, err := asl.Parse([]byte(schema))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	resolver := &asl.Resolver{}
	ir, err := resolver.Resolve(src)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	models := SchemaIRToModels(ir)
	changes := DiffSchemas(nil, models)
	up, down := GenerateMigrationSQL(changes, nil, models)

	for _, want := range []string{
		`CHECK (char_length("email") >= 6)`,
		`CHECK (char_length("email") <= 100)`,
		`CREATE INDEX IF NOT EXISTS "idx_user_email" ON "user" ("email");`,
		`CREATE INDEX IF NOT EXISTS "idx_user_active_age" ON "user" ("active", "age");`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("up SQL missing %q:\n%s", want, up)
		}
	}

	// Rollback drops the whole table (cascade removes indexes).
	if !strings.Contains(down, `DROP TABLE IF EXISTS "user" CASCADE;`) {
		t.Errorf("down SQL missing DROP TABLE:\n%s", down)
	}
}

// TestPipelineAddIndexToExistingTable confirms that adding an index to an
// already-existing model produces a standalone CREATE INDEX / DROP INDEX pair.
func TestPipelineAddIndexToExistingTable(t *testing.T) {
	base := `type User { required id: uuid { constraint pk; }; required email: str; }`
	withIndex := `type User { required id: uuid { constraint pk; }; required email: str; index on (.email); }`

	oldModels := parseToModels(t, base)
	newModels := parseToModels(t, withIndex)

	changes := DiffSchemas(oldModels, newModels)
	up, down := GenerateMigrationSQL(changes, oldModels, newModels)

	if !strings.Contains(up, `CREATE INDEX IF NOT EXISTS "idx_user_email" ON "user" ("email");`) {
		t.Errorf("up SQL missing CREATE INDEX:\n%s", up)
	}
	if !strings.Contains(down, `DROP INDEX IF EXISTS "idx_user_email";`) {
		t.Errorf("down SQL missing DROP INDEX:\n%s", down)
	}
}

func parseToModels(t *testing.T, schema string) []Model {
	t.Helper()
	src, err := asl.Parse([]byte(schema))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&asl.Resolver{}).Resolve(src)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return SchemaIRToModels(ir)
}
