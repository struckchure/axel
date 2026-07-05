package tests

import (
	"strings"
	"testing"
)

// A nullable column (deleted_at) that sorts before a required string column
// (email) reproduces the reported "converting NULL to string" bug: with
// RETURNING * the physical column order didn't match the alphabetical struct/
// Scan order. Compiled SQL must now list columns explicitly, sorted by name.
const orderSchema = `type Account {
  required id: uuid;
  required email: str;
  deleted_at: datetime;
  created_at: datetime;
}`

func TestInsertReturningIsExplicitAndSorted(t *testing.T) {
	c := compileAQL(t, orderSchema, `insert Account { email := $email };`)
	if strings.Contains(c.SQL, "RETURNING *") {
		t.Errorf("insert must not use RETURNING *:\n%s", c.SQL)
	}
	want := `RETURNING "created_at", "deleted_at", "email", "id";`
	if !strings.Contains(c.SQL, want) {
		t.Errorf("insert RETURNING not sorted/explicit, want %q:\n%s", want, c.SQL)
	}
}

func TestUpdateReturningIsExplicitAndSorted(t *testing.T) {
	c := compileAQL(t, orderSchema, `update Account filter .id = $id set { email := $email };`)
	if strings.Contains(c.SQL, "RETURNING *") {
		t.Errorf("update must not use RETURNING *:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `RETURNING "created_at", "deleted_at", "email", "id";`) {
		t.Errorf("update RETURNING not sorted/explicit:\n%s", c.SQL)
	}
}

func TestShapelessSelectColumnsSorted(t *testing.T) {
	c := compileAQL(t, orderSchema, `select Account;`)
	// Columns must appear in sorted-by-name order.
	order := []string{"created_at", "deleted_at", "email", "id"}
	last := -1
	for _, col := range order {
		idx := strings.Index(c.SQL, "."+col)
		if idx < 0 {
			t.Fatalf("column %q missing from select:\n%s", col, c.SQL)
		}
		if idx < last {
			t.Errorf("column %q out of sorted order:\n%s", col, c.SQL)
		}
		last = idx
	}
}

// The compiled SQL column order must equal the descriptor's Result.Fields order
// (both sorted by property name) so the generated positional Scan lines up.
func TestReturningMatchesDescriptorFieldOrder(t *testing.T) {
	ir := parseSchema(t, orderSchema)
	desc := buildQueryDesc(t, ir, "createAccount", "create_account.aql", `insert Account { email := $email };`)

	got := make([]string, len(desc.Result.Fields))
	for i, f := range desc.Result.Fields {
		got[i] = f.Name
	}
	want := []string{"created_at", "deleted_at", "email", "id"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("descriptor field order = %v, want %v", got, want)
	}

	// Each field, in order, appears in the RETURNING clause at a monotonically
	// increasing position — i.e. SQL column order == field order.
	c := compileAQL(t, orderSchema, `insert Account { email := $email };`)
	ret := c.SQL[strings.Index(c.SQL, "RETURNING"):]
	last := -1
	for _, name := range want {
		idx := strings.Index(ret, `"`+name+`"`)
		if idx < 0 || idx < last {
			t.Errorf("field %q not in RETURNING order:\n%s", name, ret)
		}
		last = idx
	}
}
