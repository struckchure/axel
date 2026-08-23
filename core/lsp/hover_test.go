package lsp

import (
	"strings"
	"testing"
)

func TestQueryHoverDirectives(t *testing.T) {
	schema := schemaFor(t, enumSchema)

	text := "@rel_load_strategy join\nselect Job { title };"
	// Hover over "rel_load_strategy"
	offset := strings.Index(text, "rel_load_strategy")
	h := QueryHover(text, offset, schema)
	if h == nil {
		t.Fatal("expected hover for rel_load_strategy, got nil")
	}
	if !strings.Contains(h.Contents, "@rel_load_strategy") {
		t.Errorf("hover contents missing directive name:\n%s", h.Contents)
	}

	// Hover over "join"
	offsetJoin := strings.Index(text, "join")
	hJoin := QueryHover(text, offsetJoin, schema)
	if hJoin == nil {
		t.Fatal("expected hover for join, got nil")
	}
	if !strings.Contains(hJoin.Contents, "LEFT JOIN LATERAL") {
		t.Errorf("hover contents missing strategy description:\n%s", hJoin.Contents)
	}
}

func TestQueryHoverFunctions(t *testing.T) {
	schema := schemaFor(t, funcSchema)
	text := "select User filter random_string(10);"
	offset := strings.Index(text, "random_string")
	h := QueryHover(text, offset, schema)
	if h == nil {
		t.Fatal("expected hover for function random_string, got nil")
	}
	if !strings.Contains(h.Contents, "function random_string(len: int32) -> str") {
		t.Errorf("hover contents missing function signature:\n%s", h.Contents)
	}
}

func TestSchemaHoverFunctionInDefault(t *testing.T) {
	schemaText := `
function random_hex(len: int32) -> str { return 'hex'; };

scalar type Code extends str {
  constraint min_length(6);
  constraint max_length(6);
  default := random_hex(6);
}
`
	schema := schemaFor(t, schemaText)
	offset := strings.Index(schemaText, "random_hex(6)")
	h := SchemaHover(schemaText, offset, schema)
	if h == nil {
		t.Fatal("expected hover for function random_hex inside default, got nil")
	}
	if !strings.Contains(h.Contents, "function random_hex(len: int32) -> str") {
		t.Errorf("unexpected hover contents:\n%s", h.Contents)
	}
}

func TestSchemaHoverAndQueryHoverCustomSQLScalars(t *testing.T) {
	schemaText := `
use extension 'postgis';

scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32 := ST_Y(__self__::geometry);
  longitude: float32 := ST_X(__self__::geometry);
};

type Venue {
  required id: uuid;
  location: Point;
}
`
	schema := schemaFor(t, schemaText)

	// 1. Schema hover on Point
	hPoint := SchemaHover(schemaText, strings.Index(schemaText, "Point"), schema)
	if hPoint == nil || !strings.Contains(hPoint.Contents, `scalar type Point extends sql "geography(Point, 4326)" as {`) {
		t.Fatalf("Point schema hover mismatch:\n%v", hPoint)
	}
	if !strings.Contains(hPoint.Contents, `latitude: float32 := ST_Y(__self__::geometry);`) {
		t.Errorf("Point schema hover missing field expression:\n%s", hPoint.Contents)
	}

	// 2. Query hover on .location.latitude
	qText := "select Venue { lat := .location.latitude };"
	hLat := QueryHover(qText, strings.Index(qText, "latitude"), schema)
	if hLat == nil {
		t.Fatalf("expected hover for latitude subfield, got nil")
	}
	if !strings.Contains(hLat.Contents, "latitude: float32 := ST_Y(__self__::geometry)") {
		t.Errorf("latitude subfield hover mismatch:\n%s", hLat.Contents)
	}
	if !strings.Contains(hLat.Contents, "field of `scalar type Point`") {
		t.Errorf("latitude subfield hover missing parent scalar reference:\n%s", hLat.Contents)
	}
}

func TestQueryHoverVariablesAndWithBindings(t *testing.T) {
	schemaText := `
scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32 := ST_Y(__self__::geometry);
  longitude: float32 := ST_X(__self__::geometry);
};

enum Status { Active, Inactive }

type Venue {
  required id: uuid;
  status: Status;
  location: Point;
}
`
	schema := schemaFor(t, schemaText)

	qText := `var (
  $lat<float32>;
  $status<Status>?;
  $target<Point>;
)

with (
  nearby := (select Venue filter .status = Status.Active);
)

multi select Venue { id }
filter ST_DWithin(.location, $target, 1000.0) and .status = $status;`

	// 1. Hover on $lat reference
	hLat := QueryHover(qText, strings.Index(qText, "$lat"), schema)
	if hLat == nil || !strings.Contains(hLat.Contents, "var $lat<float32>") {
		t.Fatalf("expected hover for $lat, got: %v", hLat)
	}

	// 2. Hover on $status reference in filter
	hStatus := QueryHover(qText, strings.LastIndex(qText, "$status"), schema)
	if hStatus == nil || !strings.Contains(hStatus.Contents, "var $status<Status>?") {
		t.Fatalf("expected hover for $status, got: %v", hStatus)
	}
	if !strings.Contains(hStatus.Contents, "enum Status { Active, Inactive }") {
		t.Errorf("expected enum definition in $status hover:\n%s", hStatus.Contents)
	}

	// 3. Hover on $target with custom Point scalar
	hTarget := QueryHover(qText, strings.Index(qText, "$target,"), schema)
	if hTarget == nil || !strings.Contains(hTarget.Contents, "var $target<Point>") {
		t.Fatalf("expected hover for $target, got: %v", hTarget)
	}
	if !strings.Contains(hTarget.Contents, "scalar type Point extends sql") {
		t.Errorf("expected Point scalar definition in $target hover:\n%s", hTarget.Contents)
	}

	// 4. Hover on with-binding nearby
	hNearby := QueryHover(qText, strings.Index(qText, "nearby :="), schema)
	if hNearby == nil || !strings.Contains(hNearby.Contents, "with nearby :=") {
		t.Fatalf("expected hover for nearby, got: %v", hNearby)
	}
}


