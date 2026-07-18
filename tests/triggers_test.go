package tests

import (
	"strings"
	"testing"

	axel "github.com/struckchure/axel/core"
)

// between returns the substring of s between the first occurrence of start and
// the next occurrence of end, or "" if not found.
func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	i += len(start)
	j := strings.Index(s[i:], end)
	if j < 0 {
		return ""
	}
	return s[i : i+j]
}

// genUpTriggers returns up-migration SQL from an empty baseline, including
// lowered functions and triggers.
func genUpTriggers(t *testing.T, schema string) string {
	t.Helper()
	up, _ := genMigrationTriggers(t, "", schema)
	return up
}

// genMigrationTriggers returns up/down SQL for old→new, diffing models,
// functions, and triggers (mirrors MigrationGenerator.GenerateMigration).
func genMigrationTriggers(t *testing.T, oldSchema, newSchema string) (up, down string) {
	t.Helper()
	var oldModels []axel.Model
	var oldFns []axel.Function
	var oldTrigs []axel.Trigger
	if strings.TrimSpace(oldSchema) != "" {
		ir := parseSchema(t, oldSchema)
		oldModels = axel.SchemaIRToModels(ir)
		var err error
		oldFns, oldTrigs, err = axel.SchemaIRToFunctionsAndTriggers(ir)
		if err != nil {
			t.Fatalf("lower old: %v", err)
		}
	}
	newIR := parseSchema(t, newSchema)
	newModels := axel.SchemaIRToModels(newIR)
	newFns, newTrigs, err := axel.SchemaIRToFunctionsAndTriggers(newIR)
	if err != nil {
		t.Fatalf("lower new: %v", err)
	}
	changes := axel.DiffSchemas(oldModels, newModels)
	changes = append(changes, axel.DiffFunctions(oldFns, newFns)...)
	changes = append(changes, axel.DiffTriggers(oldTrigs, newTrigs)...)
	return axel.GenerateMigrationSQL(changes, oldModels, newModels)
}

const trigSchema = `
abstract type Base {
  id: uuid { default := gen_uuid(); };
  updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();
  };
}
type AuditLog {
  id: uuid { default := gen_uuid(); };
  required table_name: str;
  action: str;
  new_data: json;
}
type Application extends Base {
  required name: str;
  trigger audit after insert, update, delete do (
    insert AuditLog { table_name := 'application', action := event, new_data := to_jsonb(__new__) }
  );
}
`

// A field rewrite (inherited from abstract Base) becomes a folded BEFORE trigger
// function referenced by a per-table trigger.
func TestRewriteBecomesTrigger(t *testing.T) {
	up := genUpTriggers(t, trigSchema)
	for _, want := range []string{
		`CREATE OR REPLACE FUNCTION "axel_rw_`,
		`IF TG_OP = 'UPDATE' THEN`,
		`NEW."updated_at" := now();`,
		`RETURN NEW;`,
		`CREATE TRIGGER "trg_application_rewrite_base" BEFORE UPDATE ON "application"`,
		`FOR EACH ROW EXECUTE FUNCTION "axel_rw_base_1"`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("rewrite DDL missing %q:\n%s", want, up)
		}
	}
	// The trigger executes the same function that was created.
	fnName := between(up, `CREATE OR REPLACE FUNCTION "`, `"() RETURNS trigger`)
	if fnName == "" || !strings.Contains(up, `EXECUTE FUNCTION "`+fnName+`"();`) {
		t.Errorf("trigger should execute the created function %q:\n%s", fnName, up)
	}
}

// A rewrite inherited from one abstract type by several concrete types produces
// exactly ONE shared function (named after the origin type), reused by each
// table's trigger.
func TestRewriteFunctionSharedAcrossTables(t *testing.T) {
	schema := `
abstract type Base {
  id: uuid { default := gen_uuid(); };
  updated_at: datetime { default := datetime_current(); rewrite update := datetime_current(); };
}
type User extends Base { required email: str; }
type Post extends Base { required title: str; }
type Comment extends Base { required body: str; }
`
	up := genUpTriggers(t, schema)
	if n := strings.Count(up, "CREATE OR REPLACE FUNCTION"); n != 1 {
		t.Errorf("expected exactly one shared rewrite function, got %d:\n%s", n, up)
	}
	// The shared function is named after the declaring (abstract) type.
	if !strings.Contains(up, `CREATE OR REPLACE FUNCTION "axel_rw_base_1"()`) {
		t.Errorf("shared function should be named after Base:\n%s", up)
	}
	for _, table := range []string{"user", "post", "comment"} {
		if !strings.Contains(up, `CREATE TRIGGER "trg_`+table+`_rewrite_base" BEFORE UPDATE ON "`+table+`"`) {
			t.Errorf("missing per-table trigger for %q:\n%s", table, up)
		}
	}
	// All three triggers execute the one shared function.
	if c := strings.Count(up, `EXECUTE FUNCTION "axel_rw_base_1"();`); c != 3 {
		t.Errorf("expected 3 triggers to share axel_rw_base_1, got %d:\n%s", c, up)
	}
}

// A rewrite inherited from an abstract parent and one declared on the concrete
// type produce SEPARATE per-model functions; the concrete table gets a trigger
// for each. The abstract's function is not duplicated into the concrete one.
func TestRewriteFunctionPerModel(t *testing.T) {
	schema := `
abstract type Base {
  id: uuid { default := gen_uuid(); };
  updated_at: datetime { rewrite update := datetime_current(); };
}
type Doc extends Base {
  required title: str;
  slug: str { rewrite insert, update := __subject__.title; };
}
`
	up := genUpTriggers(t, schema)
	// Two functions: one per declaring model.
	if !strings.Contains(up, `CREATE OR REPLACE FUNCTION "axel_rw_base_1"()`) {
		t.Errorf("Base rewrite should be its own function:\n%s", up)
	}
	if !strings.Contains(up, `CREATE OR REPLACE FUNCTION "axel_rw_doc_1"()`) {
		t.Errorf("Doc's own rewrite should be a separate function:\n%s", up)
	}
	if n := strings.Count(up, "CREATE OR REPLACE FUNCTION"); n != 2 {
		t.Errorf("expected exactly 2 rewrite functions, got %d:\n%s", n, up)
	}
	// Base's function must not contain Doc's slug assignment.
	baseFn := between(up, `FUNCTION "axel_rw_base_1"() RETURNS trigger AS $$`, `$$ LANGUAGE`)
	if strings.Contains(baseFn, "slug") {
		t.Errorf("Base's function must not duplicate Doc's rewrite:\n%s", baseFn)
	}
	// Doc gets a trigger for each contributing model.
	for _, want := range []string{
		`CREATE TRIGGER "trg_doc_rewrite_base" BEFORE UPDATE ON "doc"`,
		`CREATE TRIGGER "trg_doc_rewrite_doc" BEFORE INSERT OR UPDATE ON "doc"`,
		`EXECUTE FUNCTION "axel_rw_base_1"()`,
		`EXECUTE FUNCTION "axel_rw_doc_1"()`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("per-model trigger missing %q:\n%s", want, up)
		}
	}
}

// Multiple rewrite fields on one table fold into a single function, one IF per
// event.
func TestRewritesFoldIntoOneFunction(t *testing.T) {
	schema := `
type Doc {
  id: uuid { default := gen_uuid(); };
  updated_at: datetime { rewrite update := datetime_current(); };
  slug: str { rewrite insert, update := __subject__.title; };
  title: str;
}
`
	up := genUpTriggers(t, schema)
	if n := strings.Count(up, `CREATE OR REPLACE FUNCTION "axel_rw_`); n != 1 {
		t.Errorf("expected exactly one rewrite function, got %d:\n%s", n, up)
	}
	for _, want := range []string{
		`IF TG_OP = 'INSERT' THEN`,
		`IF TG_OP = 'UPDATE' THEN`,
		`NEW."slug" := NEW."title";`, // __subject__.title
		`BEFORE INSERT OR UPDATE ON "doc"`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("folded rewrite missing %q:\n%s", want, up)
		}
	}
}

// An inline do-body compiles to a plpgsql function with NEW refs, event→TG_OP,
// no RETURNING, and the standard return; the trigger fires on all three events.
func TestDoBodyTrigger(t *testing.T) {
	up := genUpTriggers(t, trigSchema)
	for _, want := range []string{
		`CREATE OR REPLACE FUNCTION "axel_trg_application_audit"() RETURNS trigger AS $$`,
		`INSERT INTO "audit_log"`,
		`to_jsonb(NEW)`,
		`RETURN COALESCE(NEW, OLD);`,
		`CREATE TRIGGER "trg_application_audit" AFTER INSERT OR UPDATE OR DELETE ON "application"`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("do-body DDL missing %q:\n%s", want, up)
		}
	}
	if strings.Contains(up, "RETURNING") {
		t.Errorf("compiled do-body must not keep RETURNING:\n%s", up)
	}
	// Ordering: the function must be created before the trigger that uses it.
	if strings.Index(up, `FUNCTION "axel_trg_application_audit"() RETURNS`) >
		strings.Index(up, `CREATE TRIGGER "trg_application_audit"`) {
		t.Error("function must be created before its trigger")
	}
}

// A declared function: AQL body compiles to plpgsql; raw $$ body passes through.
func TestDeclaredFunctions(t *testing.T) {
	schema := `
type AuditLog { id: uuid; required table_name: str; }
function log_it() -> trigger {
  body := ( insert AuditLog { table_name := 'x' } );
};
function raw_slug() -> trigger {
  language := plpgsql;
  body := $$ BEGIN NEW.slug := lower(NEW.name); RETURN NEW; END; $$;
};
`
	up := genUpTriggers(t, schema)
	for _, want := range []string{
		`CREATE OR REPLACE FUNCTION "log_it"() RETURNS trigger AS $$`,
		`INSERT INTO "audit_log"`,
		`CREATE OR REPLACE FUNCTION "raw_slug"() RETURNS trigger AS $$`,
		`NEW.slug := lower(NEW.name); RETURN NEW;`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("declared function DDL missing %q:\n%s", want, up)
		}
	}
}

// The execute-form references a declared function without synthesizing one.
func TestExecuteFormTrigger(t *testing.T) {
	schema := `
function touch() -> trigger { body := $$ BEGIN RETURN NEW; END; $$; };
type Widget {
  id: uuid;
  name: str;
  trigger t before insert, update execute touch();
}
`
	up := genUpTriggers(t, schema)
	if !strings.Contains(up, `CREATE TRIGGER "trg_widget_t" BEFORE INSERT OR UPDATE ON "widget"`) {
		t.Errorf("execute-form trigger line missing:\n%s", up)
	}
	if !strings.Contains(up, `EXECUTE FUNCTION "touch"();`) {
		t.Errorf("execute-form should reference the declared function:\n%s", up)
	}
	if strings.Contains(up, "axel_trg_widget_t") {
		t.Errorf("execute-form must not synthesize a function:\n%s", up)
	}
}

// Adding a trigger to an existing schema yields add up / drop down; removing it
// is the reverse; an unchanged schema produces no trigger churn.
func TestTriggerMigrationAddRemove(t *testing.T) {
	base := `
type AuditLog { id: uuid; required table_name: str; new_data: json; }
type Application extends Base { required name: str; }
abstract type Base { id: uuid; }
`
	withTrig := `
type AuditLog { id: uuid; required table_name: str; new_data: json; }
abstract type Base { id: uuid; }
type Application extends Base {
  required name: str;
  trigger audit after insert do ( insert AuditLog { table_name := 'application' } );
}
`
	// Add.
	up, down := genMigrationTriggers(t, base, withTrig)
	if !strings.Contains(up, `CREATE TRIGGER "trg_application_audit"`) ||
		!strings.Contains(up, `CREATE OR REPLACE FUNCTION "axel_trg_application_audit"`) {
		t.Errorf("add-trigger up missing CREATEs:\n%s", up)
	}
	if !strings.Contains(down, `DROP TRIGGER IF EXISTS "trg_application_audit"`) ||
		!strings.Contains(down, `DROP FUNCTION IF EXISTS "axel_trg_application_audit"`) {
		t.Errorf("add-trigger down missing DROPs:\n%s", down)
	}
	// Down drops the trigger before the function.
	if strings.Index(down, `DROP TRIGGER IF EXISTS "trg_application_audit"`) >
		strings.Index(down, `DROP FUNCTION IF EXISTS "axel_trg_application_audit"`) {
		t.Errorf("down must drop trigger before function:\n%s", down)
	}

	// Remove (reverse).
	up2, _ := genMigrationTriggers(t, withTrig, base)
	if !strings.Contains(up2, `DROP TRIGGER IF EXISTS "trg_application_audit"`) {
		t.Errorf("remove-trigger up should drop the trigger:\n%s", up2)
	}

	// Unchanged → no trigger churn.
	up3, down3 := genMigrationTriggers(t, withTrig, withTrig)
	if strings.Contains(up3, "TRIGGER") || strings.Contains(down3, "TRIGGER") {
		t.Errorf("unchanged schema should not touch triggers:\nup=%s\ndown=%s", up3, down3)
	}
}

// Editing a do-body is a ModifyFunction (CREATE OR REPLACE), not churn on the
// trigger definition.
func TestTriggerBodyModify(t *testing.T) {
	before := `
type AuditLog { id: uuid; required table_name: str; action: str; }
abstract type Base { id: uuid; }
type Application extends Base {
  required name: str;
  trigger audit after insert do ( insert AuditLog { table_name := 'application' } );
}
`
	after := `
type AuditLog { id: uuid; required table_name: str; action: str; }
abstract type Base { id: uuid; }
type Application extends Base {
  required name: str;
  trigger audit after insert do ( insert AuditLog { table_name := 'application', action := event } );
}
`
	up, _ := genMigrationTriggers(t, before, after)
	if !strings.Contains(up, `CREATE OR REPLACE FUNCTION "axel_trg_application_audit"`) {
		t.Errorf("body edit should re-create the function:\n%s", up)
	}
	if !strings.Contains(up, "TG_OP") {
		t.Errorf("new body should reference the added event column:\n%s", up)
	}
	// The trigger definition itself is unchanged, so no DROP/CREATE TRIGGER.
	if strings.Contains(up, "DROP TRIGGER") || strings.Contains(up, "CREATE TRIGGER") {
		t.Errorf("unchanged trigger definition should not be re-created:\n%s", up)
	}
}
