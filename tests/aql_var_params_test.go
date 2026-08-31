package tests

import (
	"strings"
	"testing"
)

func TestVarParamsBlock(t *testing.T) {
	c := compileAQL(t, typedParamSchema, `
var (
  $status<TransactionStatus>?;
  $min_amount<int32>?;
  $limit<int32>?;
)

multi select Transaction { id, status }
filter .status = $status and .amount >= $min_amount
limit $limit;
`)

	// Declared optional (`$status<...>?`), so each comparison is skipped when the
	// value comes in null — same as the inline `$status?` form.
	if !strings.Contains(c.SQL, "WHERE ($1::TEXT IS NULL OR t.status = $1::TEXT) AND ($2::INTEGER IS NULL OR t.amount >= $2::INTEGER)") {
		t.Errorf("unexpected WHERE clause in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "LIMIT $3::INTEGER") {
		t.Errorf("unexpected LIMIT in SQL:\n%s", c.SQL)
	}
	if len(c.Params) != 3 {
		t.Fatalf("expected 3 params, got %d (%v)", len(c.Params), c.Params)
	}

	p0 := c.Params[0]
	if p0.Name != "status" || p0.EnumType != "TransactionStatus" || !p0.Optional {
		t.Errorf("p0 unexpected: %+v", p0)
	}
	p1 := c.Params[1]
	if p1.Name != "min_amount" || p1.AQLType != "int32" || !p1.Optional {
		t.Errorf("p1 unexpected: %+v", p1)
	}
	p2 := c.Params[2]
	if p2.Name != "limit" || p2.AQLType != "int32" || !p2.Optional {
		t.Errorf("p2 unexpected: %+v", p2)
	}
}

func TestVarSingleStatements(t *testing.T) {
	c := compileAQL(t, typedParamSchema, `
var $status<TransactionStatus>?;
var $limit<int32>?;

multi select Transaction { id }
filter .status = $status
limit $limit;
`)

	if len(c.Params) != 2 {
		t.Fatalf("expected 2 params, got %d (%v)", len(c.Params), c.Params)
	}
	if c.Params[0].Name != "status" || c.Params[0].EnumType != "TransactionStatus" {
		t.Errorf("p0 unexpected: %+v", c.Params[0])
	}
	if c.Params[1].Name != "limit" || c.Params[1].AQLType != "int32" {
		t.Errorf("p1 unexpected: %+v", c.Params[1])
	}
}

func TestVarAndWithBlockCombined(t *testing.T) {
	schema := `
enum ApiKeyEnvironment { Live, Test }
type ApiKey {
  required id: uuid;
  required business_id: uuid;
  required environment: ApiKeyEnvironment;
}
type Transaction {
  required id: uuid;
  required currency: str;
  required amount: int64;
  required sender_id: uuid;
  required reciever_id: uuid;
}
`
	query := `
var (
  $currency<str>?;
  $api_key_id<uuid>?;
  $business_id<uuid>?;
  $api_environment<ApiKeyEnvironment>?;
)

with (
  api_key := (
    multi select ApiKey { id }
    filter .id = $api_key_id
    or (.business_id = $business_id and .environment = $api_environment)
  );
)
multi select Transaction {
  currency,
  total := sum(.amount)<int64>
}
filter (
  ($currency is not null and lower(.currency) = lower($currency))
  and (.sender_id in api_key.id or .reciever_id in api_key.id)
)
group by .currency;
`

	c := compileAQL(t, schema, query)

	if !strings.Contains(c.SQL, "$1::TEXT IS NOT NULL") {
		t.Errorf("expected $1::TEXT IS NOT NULL in SQL:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, "lower($1::TEXT)") {
		t.Errorf("expected lower($1::TEXT) in SQL:\n%s", c.SQL)
	}
	if len(c.Params) != 4 {
		t.Fatalf("expected 4 params, got %d (%v)", len(c.Params), c.Params)
	}
	if c.Params[0].Name != "currency" || c.Params[0].AQLType != "str" {
		t.Errorf("param 0 unexpected: %+v", c.Params[0])
	}
	if c.Params[1].Name != "api_key_id" || c.Params[1].AQLType != "uuid" {
		t.Errorf("param 1 unexpected: %+v", c.Params[1])
	}
	if c.Params[2].Name != "business_id" || c.Params[2].AQLType != "uuid" {
		t.Errorf("param 2 unexpected: %+v", c.Params[2])
	}
	if c.Params[3].Name != "api_environment" || c.Params[3].EnumType != "ApiKeyEnvironment" {
		t.Errorf("param 3 unexpected: %+v", c.Params[3])
	}
}

func TestVarRejectsObjectType(t *testing.T) {
	err := compileErr(t, typedParamSchema, `
var $t<Transaction>;
multi select Transaction { id };
`)
	if err == nil || !strings.Contains(err.Error(), "is an object type (table)") {
		t.Fatalf("expected object type rejection error, got %v", err)
	}
}

func TestVarRejectsUnknownType(t *testing.T) {
	err := compileErr(t, typedParamSchema, `
var $x<UnknownType>;
multi select Transaction { id };
`)
	if err == nil || !strings.Contains(err.Error(), "unknown parameter type") {
		t.Fatalf("expected unknown type rejection error, got %v", err)
	}
}
