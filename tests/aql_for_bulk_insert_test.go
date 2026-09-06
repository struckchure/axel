package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/compiler"
)

func TestBulkInsertForLoopWithVarMultiDefault(t *testing.T) {
	schema := `
type User {
  required id: uuid { constraint pk; };
  required email: str { constraint exclusive; };
}

type PackageCondition {
  required id: uuid { constraint pk; };
  required name: str { constraint exclusive; };
  added_by: User;
}
`
	query := `
var multi $conditions: str? := {'Hot', 'Cold', 'Fragile'};

for $condition in $conditions {
  insert PackageCondition {
    name := $condition,
    added_by := (select User filter .email = 'alice@example.com')
  } unless conflict;
}
`
	c := compileAQL(t, schema, query)

	if !strings.Contains(c.SQL, `WITH __for_iter AS (`) {
		t.Errorf("expected WITH __for_iter AS in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `SELECT unnest(COALESCE($1::TEXT[], ARRAY['Hot', 'Cold', 'Fragile']::TEXT[])) AS "condition"`) {
		t.Errorf("expected unnest with COALESCE in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `INSERT INTO "package_condition" ("name", "added_by")`) {
		t.Errorf("expected INSERT INTO package_condition in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `SELECT`) || !strings.Contains(c.SQL, `__for_iter."condition"`) {
		t.Errorf("expected select __for_iter.\"condition\" in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `(SELECT u.id FROM "user" u WHERE u.email = 'alice@example.com' LIMIT 1)`) {
		t.Errorf("expected added_by subquery in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `FROM __for_iter`) {
		t.Errorf("expected FROM __for_iter in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `ON CONFLICT DO NOTHING`) {
		t.Errorf("expected ON CONFLICT DO NOTHING in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `RETURNING "id", "name", "added_by";`) {
		t.Errorf("expected RETURNING in SQL:\n%s", c.SQL)
	}
	if len(c.Params) != 1 || c.Params[0].Name != "conditions" || !c.Params[0].Multi || !c.Params[0].Optional {
		t.Fatalf("expected multi optional 'conditions' param, got %+v", c.Params)
	}
}

func TestVarDefaultScalarParam(t *testing.T) {
	schema := `
type Item {
  required id: uuid { constraint pk; };
  required name: str;
  status: str;
}
`
	query := `
var $limit: int32? := 20;
var $status: str? := 'active';

multi select Item { id, name }
filter .status = $status
limit $limit;
`
	c := compileAQL(t, schema, query)

	if !strings.Contains(c.SQL, `COALESCE($1::INTEGER, 20)`) {
		t.Errorf("expected limit with COALESCE default in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `COALESCE($2::TEXT, 'active')`) {
		t.Errorf("expected filter with COALESCE default in SQL:\n%s", c.SQL)
	}
}

func TestForLoopWithLiteralSet(t *testing.T) {
	schema := `
type Category {
  required id: uuid { constraint pk; };
  required name: str;
}
`
	query := `
for $c in {'Electronics', 'Books', 'Clothing'} {
  insert Category {
    name := $c
  };
}
`
	c := compileAQL(t, schema, query)

	if !strings.Contains(c.SQL, `SELECT unnest(ARRAY['Electronics', 'Books', 'Clothing']) AS "c"`) {
		t.Errorf("expected unnest literal array in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `INSERT INTO "category" ("name")`) {
		t.Errorf("expected INSERT INTO category in SQL:\n%s", c.SQL)
	}
}

// A for-loop's INSERT reads its values from the __for_iter CTE, but Postgres
// puts neither that CTE nor any other FROM item in scope inside
// ON CONFLICT DO UPDATE — only the existing row and EXCLUDED. Emitting the
// iterator expression there produced SQL that compiled fine and then failed at
// run time with 42P01 (missing FROM-clause entry for table "__for_iter"), so
// the update rewrites each insert value expression to the EXCLUDED reference of
// the column the insert wrote it to.
func TestBulkInsertForLoopConflictUpdateUsesExcluded(t *testing.T) {
	schema := `
type Plan {
  required id: uuid { constraint pk; };
  required name: str { constraint exclusive; };
  required price: int64;
  required currency: str;
  required memory: int32;
}
`
	query := `
var multi $plans: str? := {'A1|0|NGN|512', 'A2|150000|NGN|1024'};

for $plan in $plans {
  insert Plan {
    name := split_part($plan, '|', 0),
    price := split_part($plan, '|', 1)<int64>,
    currency := split_part($plan, '|', 2),
    memory := split_part($plan, '|', 3)<int32>
  } unless conflict on .name else (update Plan set {
    price := split_part($plan, '|', 1)<int64>,
    memory := split_part($plan, '|', 3)<int32> + 1
  });
}
`
	c := compileAQL(t, schema, query)

	if strings.Contains(c.SQL[strings.Index(c.SQL, "ON CONFLICT"):], "__for_iter") {
		t.Errorf("ON CONFLICT clause still references __for_iter:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `DO UPDATE SET "price" = EXCLUDED."price", "memory" = EXCLUDED."memory" + 1`) {
		t.Errorf("expected EXCLUDED-based DO UPDATE SET in SQL:\n%s", c.SQL)
	}
}

// Only values the insert actually writes exist as EXCLUDED columns, so an
// update expression over the iterator that the insert never assigns has no
// legal lowering and must fail at compile time rather than at run time.
func TestBulkInsertForLoopConflictUpdateUnknownIterExpr(t *testing.T) {
	schema := `
type Plan {
  required id: uuid { constraint pk; };
  required name: str { constraint exclusive; };
  required currency: str;
}
`
	query := `
var multi $plans: str? := {'A1|NGN'};

for $plan in $plans {
  insert Plan {
    name := split_part($plan, '|', 0)
  } unless conflict on .name else (update Plan set {
    currency := split_part($plan, '|', 1)
  });
}
`
	ir := parseSchema(t, schema)
	stmt, err := aql.ParseString(query)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := compiler.Compile(stmt, ir); err == nil {
		t.Fatal("expected an error for an iterator expression the insert does not write")
	} else if !strings.Contains(err.Error(), "currency") {
		t.Fatalf("expected the error to name the field, got: %v", err)
	}
}
