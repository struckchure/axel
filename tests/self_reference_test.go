package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/lsp"
)

// A type may reference itself. This is a normal relational shape (a tree/graph
// via a self foreign key) and must resolve and validate without error.
func TestSelfReferentialLinkValidates(t *testing.T) {
	schema := `
type Transaction {
  required id: uuid;
  link parent: Transaction;
}
`
	ir := parseSchema(t, schema)
	if errs := asl.Validate(ir); len(errs) != 0 {
		t.Fatalf("self-referential type should validate cleanly, got: %v", errs)
	}
}

// Two types may reference each other (a mutual/cyclic foreign-key relationship).
func TestMutualReferenceValidates(t *testing.T) {
	schema := `
type A {
  required id: uuid;
  link b: B;
}
type B {
  required id: uuid;
  link a: A;
}
`
	ir := parseSchema(t, schema)
	if errs := asl.Validate(ir); len(errs) != 0 {
		t.Fatalf("mutually-referential types should validate cleanly, got: %v", errs)
	}
}

// A genuine inheritance cycle (A extends B, B extends A) is still an error, now
// reported at resolve time where the `extends` edges are available.
func TestInheritanceCycleRejected(t *testing.T) {
	schema := `
type A extending B { required id: uuid; }
type B extending A { required name: str; }
`
	err := resolveErr(t, schema)
	if err == nil {
		t.Fatal("expected an inheritance-cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "inheritance cycle") {
		t.Errorf("error should mention inheritance cycle, got: %v", err)
	}
}

// The migration generator must emit the self-referential table (and its FK)
// rather than dropping it during the dependency-ordering topological sort.
func TestSelfReferentialLinkMigration(t *testing.T) {
	schema := `
type Transaction {
  required id: uuid;
  link parent: Transaction;
}
`
	up := genUp(t, schema)
	if !strings.Contains(up, `CREATE TABLE "transaction"`) {
		t.Fatalf("migration should create the transaction table, got:\n%s", up)
	}
	if !strings.Contains(up, `REFERENCES "transaction"`) {
		t.Errorf("migration should contain the self-referencing foreign key, got:\n%s", up)
	}
}

// The editor path (LSP diagnostics) must not flag a self-referential schema.
func TestSelfReferentialLinkNoDiagnostics(t *testing.T) {
	schema := "type Transaction {\n  required id: uuid;\n  link parent: Transaction;\n}\n"
	if d := lsp.SchemaDiagnostics(schema); len(d) != 0 {
		t.Errorf("self-referential schema should have no diagnostics, got %+v", d)
	}
}
