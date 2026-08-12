package axel

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

// TestSnakeCaseConsistencyDigitName guards against the table-naming divergence
// that used to exist between the DDL layer (lo.SnakeCase) and the AQL
// resolver/compiler (a naive local toSnakeCase). For a digit-adjacent type name
// like Org2 the two disagreed — CREATE TABLE made "org_2" while compiled queries
// and policies referenced "org2", a dangling reference. All three must now agree.
func TestSnakeCaseConsistencyDigitName(t *testing.T) {
	src := `
global current_user: uuid;

type User { required email: str; }
type Org2 {
  required name: str;
  link owner: User;

  policy owner_only for select to app_user
    using ( .owner = global current_user );
}`

	sf, err := asl.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	const wantTable = "org_2" // lo.SnakeCase("Org2")

	// 1. The resolved IR table name.
	rt := ir.ObjectTypes["Org2"]
	if rt == nil {
		t.Fatal("Org2 not resolved")
	}
	if rt.Table != wantTable {
		t.Errorf("ResolvedType.Table = %q, want %q", rt.Table, wantTable)
	}

	// 2. The physical CREATE TABLE (DDL layer, via SchemaIRToModels + generateTable).
	models, err := SchemaIRToModels(ir)
	if err != nil {
		t.Fatalf("SchemaIRToModels: %v", err)
	}
	abstract := map[string]Model{}
	var org2 *Model
	for i := range models {
		if models[i].IsAbstract {
			abstract[models[i].Name] = models[i]
		}
		if models[i].Name == "Org2" {
			org2 = &models[i]
		}
	}
	if org2 == nil {
		t.Fatal("Org2 model not produced by SchemaIRToModels")
	}
	ddl := generateTable(*org2, abstract)
	if !strings.Contains(ddl, `CREATE TABLE "org_2"`) {
		t.Errorf("CREATE TABLE does not name %q:\n%s", wantTable, ddl)
	}

	// 3. A compiled AQL select against the type.
	stmt, err := aql.ParseString(`select Org2 { name } filter .name = 'x';`)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	compiled, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}
	if !strings.Contains(compiled.SQL, `"org_2"`) {
		t.Errorf("compiled select does not reference %q:\n%s", wantTable, compiled.SQL)
	}

	// 4. A policy USING clause on the type.
	pols, err := SchemaIRToPolicies(ir)
	if err != nil {
		t.Fatalf("lower policies: %v", err)
	}
	var found bool
	for _, p := range pols {
		if p.Name == "org_2.owner_only" {
			found = true
			if !strings.Contains(p.CreateSQL, `ON "org_2"`) {
				t.Errorf("policy not attached to %q:\n%s", wantTable, p.CreateSQL)
			}
		}
	}
	if !found {
		t.Errorf("policy org_2.owner_only not found; got %d policies", len(pols))
	}
}
