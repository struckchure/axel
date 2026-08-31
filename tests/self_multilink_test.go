package tests

import (
	"strings"
	"testing"
)

// A multi link that points back at its own type cannot name both junction
// columns after the same table — Postgres rejects the duplicate ("column
// \"product\" appears twice in primary key constraint"). The target side falls
// back to the link name.
func TestSelfReferentialJunctionColumnsAreDistinct(t *testing.T) {
	up := genUp(t, `
type Product {
  required id: uuid { constraint pk; };
  multi link addons: Product;
}
`)
	for _, want := range []string{
		`"product" UUID NOT NULL`,
		`"addons" UUID NOT NULL`,
		`CONSTRAINT "pk_product_addons" PRIMARY KEY ("product", "addons")`,
		`CONSTRAINT "fk_product_addons_product" FOREIGN KEY ("product") REFERENCES "product"("id")`,
		`CONSTRAINT "fk_product_addons_addons" FOREIGN KEY ("addons") REFERENCES "product"("id")`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("self-referential junction table missing %q:\n%s", want, up)
		}
	}
	if strings.Contains(up, `PRIMARY KEY ("product", "product")`) {
		t.Errorf("junction table repeats the owner column:\n%s", up)
	}
}

// The compiled AQL must read the same two columns the DDL created.
func TestSelfReferentialMultiLinkCompilesDistinctColumns(t *testing.T) {
	schema := `
type Product {
  required id: uuid;
  required name: str;
  multi link addons: Product;
}
`
	c := compileAQL(t, schema, `select Product { id, addons: { id } };`)
	for _, want := range []string{
		`ON p_addons.id = jt_addons.addons`,
		`WHERE jt_addons.product = p.id`,
	} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("self-referential multi-link SQL missing %q:\n%s", want, c.SQL)
		}
	}
}
