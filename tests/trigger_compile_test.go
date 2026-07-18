package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/compiler"
)

const trigCompileSchema = `
type AuditLog {
  required id: uuid;
  required table_name: str;
  action: str;
  new_data: json;
  actor: uuid;
}
type Application {
  required id: uuid;
  required name: str;
  updated_by: uuid;
}
`

// A do-body insert compiles with __new__/event/whole-row refs mapped to
// NEW/OLD/TG_OP, RETURNING stripped.
func TestCompileTriggerBodyInsert(t *testing.T) {
	ir := parseSchema(t, trigCompileSchema)
	stmt, err := aql.ParseString(`insert AuditLog {
	  table_name := 'application',
	  action := event,
	  new_data := to_jsonb(__new__),
	  actor := __new__.updated_by
	}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	enclosing := ir.ObjectTypes["Application"]
	sql, err := compiler.CompileTriggerBody(stmt, ir, enclosing, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, want := range []string{
		`INSERT INTO "audit_log"`,
		`'application'`,
		`TG_OP`,            // event
		`to_jsonb(NEW)`,    // whole-row ref
		`NEW."updated_by"`, // validated field ref
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("compiled trigger body missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "RETURNING") {
		t.Errorf("RETURNING should be stripped:\n%s", sql)
	}
}

// An unknown field on __new__ is a compile error, validated against the
// enclosing type.
func TestCompileTriggerBodyUnknownField(t *testing.T) {
	ir := parseSchema(t, trigCompileSchema)
	stmt, err := aql.ParseString(`insert AuditLog { actor := __new__.nope }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = compiler.CompileTriggerBody(stmt, ir, ir.ObjectTypes["Application"], nil)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("expected unknown-field error, got %v", err)
	}
}

// A standalone function (no enclosing type) passes __new__.field through
// snake-cased without validation.
func TestCompileTriggerBodyStandalone(t *testing.T) {
	ir := parseSchema(t, trigCompileSchema)
	stmt, err := aql.ParseString(`insert AuditLog { actor := __new__.someRandomThing }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sql, err := compiler.CompileTriggerBody(stmt, ir, nil, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(sql, `NEW."some_random_thing"`) {
		t.Errorf("standalone field passthrough:\n%s", sql)
	}
}
