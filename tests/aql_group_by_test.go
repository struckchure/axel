package tests

import (
	"strings"
	"testing"
)

func TestGroupByBasic(t *testing.T) {
	c := compileAQL(t, aggSchema, `multi select Transaction {
	  status,
	  total_amount := sum(.amount)<int64>,
	  count := count()
	}
	group by .status;`)

	if !strings.Contains(c.SQL, "SELECT\n  t.status AS status,\n  (SUM(t.amount))::BIGINT AS total_amount,\n  COUNT(*) AS count") {
		t.Errorf("unexpected columns in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "FROM \"transaction\" t") {
		t.Errorf("missing FROM in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "GROUP BY t.status") {
		t.Errorf("missing GROUP BY in SQL:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "LIMIT") {
		t.Errorf("multi select without limit should not emit LIMIT:\n%s", c.SQL)
	}
}

func TestGroupByMultipleColumns(t *testing.T) {
	c := compileAQL(t, aggSchema, `multi select Transaction {
	  status,
	  type,
	  total := sum(.amount)<int64>,
	  tx_count := count()
	}
	group by .status, .type;`)

	if !strings.Contains(c.SQL, "GROUP BY t.status, t.type") {
		t.Errorf("expected multi-column GROUP BY, got:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "t.status AS status") || !strings.Contains(c.SQL, "t.type AS type") {
		t.Errorf("expected both group columns in SELECT, got:\n%s", c.SQL)
	}
}

func TestGroupByWithFilterAndHaving(t *testing.T) {
	c := compileAQL(t, aggSchema, `multi select Transaction {
	  status,
	  total_volume := sum(.amount)<int64>,
	  successful_volume := sum(.amount)<int64> filter .status = TransactionStatus.Successful,
	  order_count := count()
	}
	filter .amount >= $min_amount
	group by .status
	having count() >= $min_orders and sum(.amount) > $min_volume
	order by total_volume desc
	limit $limit;`)

	if !strings.Contains(c.SQL, "WHERE t.amount >= $1") {
		t.Errorf("missing or incorrect WHERE clause:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "GROUP BY t.status") {
		t.Errorf("missing GROUP BY clause:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "HAVING COUNT(*) >= $2 AND SUM(t.amount) > $3") {
		t.Errorf("missing or incorrect HAVING clause:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "ORDER BY total_volume DESC") {
		t.Errorf("missing ORDER BY clause:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "LIMIT $4") {
		t.Errorf("missing LIMIT clause:\n%s", c.SQL)
	}
	if len(c.Params) != 4 {
		t.Errorf("expected 4 params, got %d (%v)", len(c.Params), c.Params)
	}
}

func TestGroupBySingleRowSelect(t *testing.T) {
	c := compileAQL(t, aggSchema, `select Transaction {
	  status,
	  total := sum(.amount)<int64>
	}
	group by .status
	order by total desc;`)

	if !strings.Contains(c.SQL, "LIMIT 1") {
		t.Errorf("single select with group by should emit LIMIT 1:\n%s", c.SQL)
	}
}

func TestGroupByRejectsUngroupedField(t *testing.T) {
	err := compileErr(t, aggSchema, `multi select Transaction {
	  status,
	  sender_id,
	  total := sum(.amount)
	}
	group by .status;`)

	if err == nil || !strings.Contains(err.Error(), "must appear in the GROUP BY clause") {
		t.Fatalf("expected ungrouped field error, got %v", err)
	}
}

func TestGroupByRejectsWildcardSplat(t *testing.T) {
	err := compileErr(t, aggSchema, `multi select Transaction {
	  *
	}
	group by .status;`)

	if err == nil || !strings.Contains(err.Error(), "wildcard '*' is not allowed in a grouped select") {
		t.Fatalf("expected wildcard rejection error, got %v", err)
	}
}

func TestHavingWithoutGroupByRejection(t *testing.T) {
	err := compileErr(t, aggSchema, `multi select Transaction {
	  status
	}
	having count() > 1;`)

	if err == nil || !strings.Contains(err.Error(), "HAVING clause requires a GROUP BY clause") {
		t.Fatalf("expected HAVING requires GROUP BY error, got %v", err)
	}
}

func TestGroupByTypeInference(t *testing.T) {
	ir := parseSchema(t, aggSchema)
	desc := buildQueryDesc(t, ir, "GroupedTx", "q.aql", `multi select Transaction {
	  status,
	  n         := count(),
	  typed_sum := sum(.amount)<int64>,
	}
	group by .status;`)

	if !desc.Result.IsMultiple {
		t.Errorf("multi select should have IsMultiple=true")
	}

	fields := map[string]string{}
	enums := map[string]string{}
	for _, f := range desc.Result.Fields {
		fields[f.Name] = f.AQLType
		enums[f.Name] = f.EnumType
	}

	if enums["status"] != "TransactionStatus" {
		t.Errorf("status should have EnumType TransactionStatus, got %q", enums["status"])
	}
	if fields["n"] != "int64" {
		t.Errorf("count should be int64, got %q", fields["n"])
	}
	if fields["typed_sum"] != "int64" {
		t.Errorf("typed_sum should be int64, got %q", fields["typed_sum"])
	}
}
