package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
)

// The Go generator must emit pgx (struct scanning + runner.DBTX) and row
// structs with both json and db tags — not database/sql.
func TestGoCodegenUsesPgx(t *testing.T) {
	ir := parseSchema(t, paramSchema)
	schema := codegen.FromSchemaIR(ir)

	q := buildQueryDesc(t, ir, "listUser", "list_user.aql", `multi select User { id, email };`)
	del := buildQueryDesc(t, ir, "delUser", "del_user.aql", `delete User filter .id = $id;`)

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q, del}, &golang.GoGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	out := readFile(t, filepath.Join(dir, "list_user.go"))
	for _, want := range []string{
		"pgx.RowToStructByName[ListUserRow]",
		"db *pgxpool.Pool",
		"`json:\"id\" db:\"id\"`",
		"github.com/jackc/pgx/v5/pgxpool",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated query missing %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{"database/sql", "sql.DB", "json.Unmarshal", "QueryContext"} {
		if strings.Contains(out, bad) {
			t.Errorf("generated query should not contain %q:\n%s", bad, out)
		}
	}

	// delete uses Exec, not database/sql
	delOut := readFile(t, filepath.Join(dir, "del_user.go"))
	if !strings.Contains(delOut, "db.Exec(ctx, query") {
		t.Errorf("delete should use db.Exec:\n%s", delOut)
	}

	// runner.go migrated too
	runnerOut := readFile(t, filepath.Join(dir, "runner.go"))
	if strings.Contains(runnerOut, "database/sql") {
		t.Errorf("runner.go should not import database/sql:\n%s", runnerOut)
	}
	if !strings.Contains(runnerOut, "func NewRunner(db *pgxpool.Pool)") {
		t.Errorf("NewRunner should take *pgxpool.Pool:\n%s", runnerOut)
	}
}
