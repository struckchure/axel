package tests

import (
	"strings"
	"testing"
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
    added_by := (select User filter .email = 'ameenmohammed2311@gmail.com')
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
	if !strings.Contains(c.SQL, `(SELECT u.id FROM "user" u WHERE u.email = 'ameenmohammed2311@gmail.com' LIMIT 1)`) {
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
