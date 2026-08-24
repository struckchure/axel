package tests

import (
	"strings"
	"testing"
)

func TestMigrationJsonTypeChangeUsingClause(t *testing.T) {
	before := `
type Application {
  required id: uuid { constraint pk; };
  config: json;
  metadata: jsonb;
}
`
	after := `
type Application {
  required id: uuid { constraint pk; };
  config: jsonb;
  metadata: json;
}
`
	up, down := genMigration(t, before, after)

	// Up statements:
	// config altered to JSONB with USING
	if !strings.Contains(up, `ALTER TABLE "application" ALTER COLUMN "config" TYPE JSONB USING "config"::JSONB;`) {
		t.Errorf("expected JSONB with USING in up migration, got:\n%s", up)
	}
	// metadata altered to JSON with USING
	if !strings.Contains(up, `ALTER TABLE "application" ALTER COLUMN "metadata" TYPE JSON USING "metadata"::JSON;`) {
		t.Errorf("expected JSON with USING in up migration, got:\n%s", up)
	}

	// Down statements:
	if !strings.Contains(down, `ALTER TABLE "application" ALTER COLUMN "config" TYPE JSON USING "config"::JSON;`) {
		t.Errorf("expected JSON with USING in down migration, got:\n%s", down)
	}
	if !strings.Contains(down, `ALTER TABLE "application" ALTER COLUMN "metadata" TYPE JSONB USING "metadata"::JSONB;`) {
		t.Errorf("expected JSONB with USING in down migration, got:\n%s", down)
	}
}

func TestMigrationScalarTypeChangeUsingClause(t *testing.T) {
	before := `
type Event {
  required id: uuid { constraint pk; };
  payload: str;
  count: int32;
}
`
	after := `
type Event {
  required id: uuid { constraint pk; };
  payload: json;
  count: int64;
}
`
	up, _ := genMigration(t, before, after)

	if !strings.Contains(up, `ALTER TABLE "event" ALTER COLUMN "payload" TYPE JSON USING "payload"::JSON;`) {
		t.Errorf("expected payload TYPE JSON USING payload::JSON in up, got:\n%s", up)
	}
	if !strings.Contains(up, `ALTER TABLE "event" ALTER COLUMN "count" TYPE BIGINT USING "count"::BIGINT;`) {
		t.Errorf("expected count TYPE BIGINT USING count::BIGINT in up, got:\n%s", up)
	}
}
