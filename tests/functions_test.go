package tests

import (
	"strings"
	"testing"

	axel "github.com/struckchure/axel/core"
)

// genMigrationFull returns up/down SQL for old→new, diffing extensions, models,
// functions, and triggers (mirrors MigrationGenerator.GenerateMigration exactly,
// including the extensions-first ordering).
func genMigrationFull(t *testing.T, oldSchema, newSchema string) (up, down string) {
	t.Helper()
	var oldModels []axel.Model
	var oldFns []axel.Function
	var oldTrigs []axel.Trigger
	var oldExts []axel.Extension
	if strings.TrimSpace(oldSchema) != "" {
		ir := parseSchema(t, oldSchema)
		oldModels = axel.SchemaIRToModels(ir)
		var err error
		oldFns, oldTrigs, err = axel.SchemaIRToFunctionsAndTriggers(ir)
		if err != nil {
			t.Fatalf("lower old: %v", err)
		}
		oldExts = axel.SchemaIRToExtensions(ir)
	}
	newIR := parseSchema(t, newSchema)
	newModels := axel.SchemaIRToModels(newIR)
	newFns, newTrigs, err := axel.SchemaIRToFunctionsAndTriggers(newIR)
	if err != nil {
		t.Fatalf("lower new: %v", err)
	}
	newExts := axel.SchemaIRToExtensions(newIR)

	changes := axel.DiffExtensions(oldExts, newExts)
	changes = append(changes, axel.DiffSchemas(oldModels, newModels)...)
	changes = append(changes, axel.DiffFunctions(oldFns, newFns)...)
	changes = append(changes, axel.DiffTriggers(oldTrigs, newTrigs)...)
	return axel.GenerateMigrationSQL(changes, oldModels, newModels)
}

func genUpFull(t *testing.T, schema string) string {
	t.Helper()
	up, _ := genMigrationFull(t, "", schema)
	return up
}

// The canonical general-purpose function: params, a raw Postgres return type,
// attribute directives, and a bare `return <expr>;` body — plus a `use extension`.
func TestSlugifyFunction(t *testing.T) {
	schema := `
use extension 'unaccent';

@language plpgsql
@immutable
@strict
@parallel safe
function slugify(value: text) -> text {
  return regexp_replace(
    regexp_replace(lower(public.unaccent(value)), '[^a-z0-9\-_]+', '-', 'gi'),
    '(^-+|-+$)', '', 'g'
  );
};
`
	up, down := genMigrationFull(t, "", schema)

	for _, want := range []string{
		`CREATE EXTENSION IF NOT EXISTS "unaccent";`,
		`CREATE OR REPLACE FUNCTION "slugify"(value text) RETURNS text AS $$`,
		"BEGIN\n  RETURN regexp_replace(",
		`$$ LANGUAGE plpgsql IMMUTABLE STRICT PARALLEL SAFE;`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("up migration missing %q:\n%s", want, up)
		}
	}

	// Extension must be created before the function that uses it.
	if ei, fi := strings.Index(up, "CREATE EXTENSION"), strings.Index(up, "CREATE OR REPLACE FUNCTION"); ei < 0 || fi < 0 || ei > fi {
		t.Errorf("extension should precede the function:\n%s", up)
	}

	for _, want := range []string{
		`DROP FUNCTION IF EXISTS "slugify"(text);`,
		`DROP EXTENSION IF EXISTS "unaccent";`,
	} {
		if !strings.Contains(down, want) {
			t.Errorf("down migration missing %q:\n%s", want, down)
		}
	}
	// On the way down, the function is dropped before the extension.
	if fi, ei := strings.Index(down, "DROP FUNCTION"), strings.Index(down, "DROP EXTENSION"); fi < 0 || ei < 0 || fi > ei {
		t.Errorf("function should drop before the extension on down:\n%s", down)
	}
}

// A `sql`-language function uses a bare SELECT wrapper instead of BEGIN/RETURN.
func TestSqlLanguageReturnFunction(t *testing.T) {
	schema := `
@language sql
@immutable
function add(a: int32, b: int32) -> int32 {
  return a + b;
};
`
	up := genUpFull(t, schema)
	for _, want := range []string{
		`CREATE OR REPLACE FUNCTION "add"(a INTEGER, b INTEGER) RETURNS INTEGER AS $$`,
		`SELECT a + b;`,
		`$$ LANGUAGE sql IMMUTABLE;`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("sql function missing %q:\n%s", want, up)
		}
	}
}

// Array param/return types pass straight through to Postgres.
func TestArrayTypesPassthrough(t *testing.T) {
	schema := `
@language sql
function first_tag(tags: text[]) -> text {
  return tags[1];
};
`
	up, down := genMigrationFull(t, "", schema)
	if !strings.Contains(up, `CREATE OR REPLACE FUNCTION "first_tag"(tags text[]) RETURNS text AS $$`) {
		t.Errorf("array param not passed through:\n%s", up)
	}
	if !strings.Contains(down, `DROP FUNCTION IF EXISTS "first_tag"(text[]);`) {
		t.Errorf("array arg type missing from drop:\n%s", down)
	}
}

// A declared extension is not re-emitted when it is unchanged between migrations.
func TestExtensionDiffNoChange(t *testing.T) {
	schema := `use extension 'unaccent';
type Widget { id: uuid; name: str; }`
	_, _ = genMigrationFull(t, "", schema) // baseline
	up, _ := genMigrationFull(t, schema, schema)
	if strings.Contains(up, "CREATE EXTENSION") {
		t.Errorf("unchanged extension should not re-emit:\n%s", up)
	}
}
