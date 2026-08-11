package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
)

const aggSchema = `
enum TransactionType { Debit, Credit }
enum TransactionStatus { Successful, Pending }
enum TransactionActorEntity { ApiKey, User }
type Transaction {
  required id: uuid;
  required amount: int64;
  required type: TransactionType;
  required status: TransactionStatus;
  required sender_id: uuid;
  required reciever_id: uuid;
  required sender_entity: TransactionActorEntity;
  required reciever_entity: TransactionActorEntity;
}
`

// The motivating query: four conditional sums over one API key. It must lower to
// a single scan with per-column FILTER (WHERE ...) and the shared actor predicate
// as the outer WHERE — no correlated subqueries, no LIMIT.
func TestAggSelectSingleScan(t *testing.T) {
	c := compileAQL(t, aggSchema, `select Transaction {
	  success_debit  := sum(.amount)<int64> filter .type = TransactionType.Debit  and .status = TransactionStatus.Successful,
	  pending_debit  := sum(.amount)<int64> filter .type = TransactionType.Debit  and .status = TransactionStatus.Pending,
	  success_credit := sum(.amount)<int64> filter .type = TransactionType.Credit and .status = TransactionStatus.Successful,
	  pending_credit := sum(.amount)<int64> filter .type = TransactionType.Credit and .status = TransactionStatus.Pending,
	}
	filter (.sender_id = $api_key_id and .sender_entity = TransactionActorEntity.ApiKey)
	    or (.reciever_id = $api_key_id and .reciever_entity = TransactionActorEntity.ApiKey);`)

	// One FROM, one scan.
	if n := strings.Count(c.SQL, "FROM "); n != 1 {
		t.Fatalf("expected a single FROM (one scan), got %d:\n%s", n, c.SQL)
	}
	// Four FILTERed aggregates.
	if n := strings.Count(c.SQL, "FILTER (WHERE"); n != 4 {
		t.Errorf("expected 4 FILTER clauses, got %d:\n%s", n, c.SQL)
	}
	if !strings.Contains(c.SQL, "(SUM(t.amount) FILTER (WHERE t.type = 'Debit' AND t.status = 'Successful'))::BIGINT AS success_debit") {
		t.Errorf("first column shape unexpected:\n%s", c.SQL)
	}
	// Shared predicate hoisted to the outer WHERE, grouping honoured.
	if !strings.Contains(c.SQL, "WHERE (t.sender_id = $1 AND t.sender_entity = 'ApiKey') OR (t.reciever_id = $1 AND t.reciever_entity = 'ApiKey')") {
		t.Errorf("shared filter not in outer WHERE:\n%s", c.SQL)
	}
	// A single-row aggregate needs no LIMIT.
	if strings.Contains(c.SQL, "LIMIT") {
		t.Errorf("aggregate select should not emit LIMIT:\n%s", c.SQL)
	}
	// The one param is reused, not duplicated.
	if len(c.Params) != 1 || c.Params[0].Name != "api_key_id" {
		t.Errorf("expected single reused param api_key_id, got %v", c.Params)
	}
}

// count() lowers to COUNT(*); a bare aggregate without a per-field filter is fine.
func TestAggSelectCountStar(t *testing.T) {
	c := compileAQL(t, aggSchema, `select Transaction {
	  total := count(),
	  pending := count() filter .status = TransactionStatus.Pending,
	};`)
	if !strings.Contains(c.SQL, "COUNT(*) AS total") {
		t.Errorf("count() should be COUNT(*):\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "COUNT(*) FILTER (WHERE t.status = 'Pending') AS pending") {
		t.Errorf("filtered count() unexpected:\n%s", c.SQL)
	}
}

// Mixing an aggregate field with a plain row field is rejected (SQL would need a
// GROUP BY). The aggregate field triggers aggregation mode; the row field fails.
func TestAggSelectRejectsMixedShape(t *testing.T) {
	err := compileErr(t, aggSchema, `select Transaction {
	  id,
	  total := sum(.amount),
	};`)
	if err == nil || !strings.Contains(err.Error(), "aggregate") {
		t.Fatalf("expected all-aggregate error, got %v", err)
	}
}

// multi / order by / limit are meaningless on a single-row aggregate select.
func TestAggSelectRejectsMultiAndOrdering(t *testing.T) {
	if err := compileErr(t, aggSchema, `multi select Transaction { total := sum(.amount) };`); err == nil || !strings.Contains(err.Error(), "multi") {
		t.Errorf("expected multi rejection, got %v", err)
	}
	if err := compileErr(t, aggSchema, `select Transaction { total := sum(.amount) } order by .amount;`); err == nil {
		t.Errorf("expected order-by rejection")
	}
}

// The old top-level count() form still works; sum/avg/min/max over a whole type
// is rejected with a pointer to the aggregate-shape form (closes the previous
// silent `sum(*)` passthrough).
func TestTopLevelAggAllowlist(t *testing.T) {
	if c := compileAQL(t, aggSchema, `select count(Transaction filter .status = TransactionStatus.Pending);`); !strings.Contains(c.SQL, "COUNT(*)") {
		t.Errorf("top-level count still expected:\n%s", c.SQL)
	}
	err := compileErr(t, aggSchema, `select sum(Transaction filter .status = TransactionStatus.Pending);`)
	if err == nil || !strings.Contains(err.Error(), "aggregate shape") {
		t.Fatalf("expected sum(Type) rejection pointing to aggregate shape, got %v", err)
	}
}

// Type inference: count → int64; min/max preserve the column type; sum/avg need a
// cast (Postgres changes their type) and otherwise warn; an explicit cast wins.
// Every aggregate field is nullable.
func TestAggSelectTypeInference(t *testing.T) {
	ir := parseSchema(t, aggSchema)
	desc := buildQueryDesc(t, ir, "Q", "q.aql", `select Transaction {
	  n         := count(),
	  biggest   := max(.amount),
	  smallest  := min(.amount),
	  raw_sum   := sum(.amount),
	  typed_sum := sum(.amount)<int64>,
	};`)

	types := map[string]string{}
	nullable := map[string]bool{}
	for _, f := range desc.Result.Fields {
		types[f.Name] = f.AQLType
		nullable[f.Name] = f.IsNullable
	}
	if types["n"] != "int64" {
		t.Errorf("count → int64, got %q", types["n"])
	}
	if types["biggest"] != "int64" || types["smallest"] != "int64" {
		t.Errorf("min/max should preserve int64, got max=%q min=%q", types["biggest"], types["smallest"])
	}
	if types["typed_sum"] != "int64" {
		t.Errorf("cast should type sum as int64, got %q", types["typed_sum"])
	}
	if types["raw_sum"] != "json" {
		t.Errorf("uncast sum should fall back to any/json, got %q", types["raw_sum"])
	}
	for name := range types {
		if !nullable[name] {
			t.Errorf("aggregate field %q should be nullable", name)
		}
	}
	// The uncast sum warns and suggests a cast.
	var warned bool
	for _, w := range desc.Warnings {
		if strings.Contains(w, "raw_sum") && strings.Contains(w, "cast") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a cast warning for raw_sum, got %v", desc.Warnings)
	}
	// A single aggregate row, not a list.
	if desc.Result.IsMultiple {
		t.Errorf("aggregate select is a single row, not multiple")
	}
}

// The per-field filter round-trips through the printer.
func TestAggSelectPrinterRoundTrip(t *testing.T) {
	src := `select Transaction { total := sum(.amount) filter .status = TransactionStatus.Pending };`
	stmt, err := aql.ParseString(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := aql.Print(stmt)
	if !strings.Contains(out, "sum(.amount) filter .status = TransactionStatus.Pending") {
		t.Errorf("printer dropped the per-field filter:\n%s", out)
	}
	// Re-parsing the printed form succeeds and preserves the aggregate filter.
	stmt2, err := aql.ParseString(out)
	if err != nil {
		t.Fatalf("re-parse printed form: %v\n%s", err, out)
	}
	if stmt2.Select.Body.Shape.Fields[0].AggFilter == nil {
		t.Errorf("round-trip lost AggFilter")
	}
}
