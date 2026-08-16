package tests

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/struckchure/axel/core"
	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
)

func TestCodegenConfigYAMLUnmarshal(t *testing.T) {
	raw := `
package-name: ./ax
schema-path: ./schema.asl
database-url: postgres://localhost:5432/mydb
rel-load-strategy: join

codegen:
  generator: go
  out-dir: ./db/generated
  queries:
    - ./queries/*.aql
  options:
    package: generated
    client: pg
`
	var cfg axel.MigrationConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}

	if cfg.RelLoadStrategy != "join" {
		t.Errorf("RelLoadStrategy = %q, want 'join'", cfg.RelLoadStrategy)
	}
	if cfg.Codegen == nil {
		t.Fatalf("Codegen is nil")
	}
	if cfg.Codegen.Generator != "go" {
		t.Errorf("Codegen.Generator = %q, want 'go'", cfg.Codegen.Generator)
	}
	if cfg.Codegen.OutDir != "./db/generated" {
		t.Errorf("Codegen.OutDir = %q, want './db/generated'", cfg.Codegen.OutDir)
	}
	if len(cfg.Codegen.Queries) != 1 || cfg.Codegen.Queries[0] != "./queries/*.aql" {
		t.Errorf("Codegen.Queries = %v, want ['./queries/*.aql']", cfg.Codegen.Queries)
	}
	if cfg.Codegen.Options["package"] != "generated" || cfg.Codegen.Options["client"] != "pg" {
		t.Errorf("Codegen.Options = %v", cfg.Codegen.Options)
	}
}

func TestDirectiveRelLoadStrategyValidation(t *testing.T) {
	ir := parseSchema(t, directiveSchema)
	schema := codegen.FromSchemaIR(ir)

	// Valid strategy
	qValid := buildQueryDesc(t, ir, "listUsers", "list_users.aql", "@rel_load_strategy join\nmulti select User { id, email };")
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{qValid}, &golang.GoGenerator{}, &codegen.Context{OutDir: t.TempDir()}); err != nil {
		t.Fatalf("valid rel_load_strategy should pass, got: %v", err)
	}

	// Invalid strategy in query descriptor
	qInvalid := buildQueryDesc(t, ir, "listUsers", "list_users.aql", "multi select User { id, email };")
	qInvalid.Directives = map[string]string{"rel_load_strategy": "unsupported"}
	err := codegen.Walk(schema, []codegen.QueryDescriptor{qInvalid}, &golang.GoGenerator{}, &codegen.Context{OutDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected error for invalid @rel_load_strategy")
	}
	if !strings.Contains(err.Error(), "invalid rel_load_strategy") {
		t.Errorf("expected invalid rel_load_strategy error, got: %v", err)
	}
}
