package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core"
	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

func TestCustomScalarReceiverFunctionsResolve(t *testing.T) {
	src := `
use extension 'postgis';

scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32;
  longitude: float32;
};

function (p Point) deserialize() Point {
  return Point{latitude: ST_Y(p::geometry), longitude: ST_X(p::geometry)};
};

function (p Point) serialize() {
  return ST_SetSRID(ST_MakePoint(p.longitude, p.latitude), 4326);
};

abstract type Base {
  required id: uuid { default := gen_uuid(); constraint pk; };
}

type Location extends Base {
  required name: str;
  point: Point;
}
`
	sf, err := asl.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}

	ptScalar, ok := ir.ScalarTypes["Point"]
	if !ok {
		t.Fatal("Point scalar not found in SchemaIR")
	}

	if len(ptScalar.Fields) != 2 {
		t.Fatalf("expected 2 scalar fields, got %d", len(ptScalar.Fields))
	}

	latField, ok := ptScalar.Fields["latitude"]
	if !ok {
		t.Fatal("Point.latitude field missing")
	}
	if !strings.Contains(latField.ExprSQL, "ST_Y(__self__::geometry)") {
		t.Errorf("expected latitude ExprSQL to contain ST_Y(__self__::geometry), got %q", latField.ExprSQL)
	}

	lonField, ok := ptScalar.Fields["longitude"]
	if !ok {
		t.Fatal("Point.longitude field missing")
	}
	if !strings.Contains(lonField.ExprSQL, "ST_X(__self__::geometry)") {
		t.Errorf("expected longitude ExprSQL to contain ST_X(__self__::geometry), got %q", lonField.ExprSQL)
	}

	if !strings.Contains(ptScalar.SerializeSQL, "ST_SetSRID(ST_MakePoint(p.longitude, p.latitude), 4326)") {
		t.Errorf("expected SerializeSQL to be captured, got %q", ptScalar.SerializeSQL)
	}
	if ptScalar.ReceiverName != "p" {
		t.Errorf("expected ReceiverName to be 'p', got %q", ptScalar.ReceiverName)
	}

	fns, _, err := axel.SchemaIRToFunctionsAndTriggers(ir)
	if err != nil {
		t.Fatalf("SchemaIRToFunctionsAndTriggers error: %v", err)
	}

	for _, fn := range fns {
		if fn.Name == "point_serialize" || fn.Name == "point_deserialize" {
			t.Errorf("scalar codec function %q should not be emitted as a migration stored procedure", fn.Name)
		}
	}
}

func TestCustomScalarCompileQuery(t *testing.T) {
	src := `
use extension 'postgis';

scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32;
  longitude: float32;
};

function (p Point) deserialize() Point {
  return Point{latitude: ST_Y(p::geometry), longitude: ST_X(p::geometry)};
};

function (p Point) serialize() {
  return ST_SetSRID(ST_MakePoint(p.longitude, p.latitude), 4326);
};

abstract type Base {
  required id: uuid { default := gen_uuid(); constraint pk; };
}

type Location extends Base {
  required name: str;
  point: Point;
}
`
	sf, err := asl.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}

	// Test SELECT query expanding .point.latitude and .point.longitude
	queryAQL := `multi select Location { id, name, point: { latitude, longitude } };`
	stmt, err := aql.Parse([]byte(queryAQL))
	if err != nil {
		t.Fatalf("parse query error: %v", err)
	}
	compiled, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile query error: %v", err)
	}

	sql := compiled.SQL
	if !strings.Contains(sql, "ST_Y(l.point::geometry)") {
		t.Errorf("expected compiled SQL to contain ST_Y(l.point::geometry), got:\n%s", sql)
	}
	if !strings.Contains(sql, "ST_X(l.point::geometry)") {
		t.Errorf("expected compiled SQL to contain ST_X(l.point::geometry), got:\n%s", sql)
	}
}

func TestCustomScalarBackwardsCompatibility(t *testing.T) {
	src := `
use extension 'postgis';

scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32 := ST_Y(__self__::geometry);
  longitude: float32 := ST_X(__self__::geometry);
};

abstract type Base {
  required id: uuid { default := gen_uuid(); constraint pk; };
}

type Location extends Base {
  required name: str;
  point: Point;
}
`
	sf, err := asl.Parse([]byte(src))
	if err != nil {
		t.Fatalf("legacy parse error: %v", err)
	}

	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("legacy resolve error: %v", err)
	}

	queryAQL := `select Location { id, point: { latitude, longitude } } filter .id = $id;`
	stmt, err := aql.Parse([]byte(queryAQL))
	if err != nil {
		t.Fatalf("legacy parse query error: %v", err)
	}
	compiled, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatalf("legacy compile query error: %v", err)
	}

	sql := compiled.SQL
	if !strings.Contains(sql, "ST_Y(l.point::geometry)") || !strings.Contains(sql, "ST_X(l.point::geometry)") {
		t.Errorf("legacy query expansion failed:\n%s", sql)
	}
}

func TestCustomScalarInsertUpdateSerialization(t *testing.T) {
	src := `
use extension 'postgis';

scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32;
  longitude: float32;
};

function (p Point) deserialize() Point {
  return Point{ latitude: ST_Y(p::geometry), longitude: ST_X(p::geometry) };
};

function (p Point) serialize() {
  return ST_SetSRID(ST_MakePoint(p.longitude, p.latitude), 4326);
};

abstract type Base {
  required id: uuid { default := gen_uuid(); constraint pk; };
}

type Location extends Base {
  required name: str;
  point: Point;
}
`
	sf, err := asl.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}

	// 1. Test INSERT serialization
	insertAQL := `insert Location { name := $name, point := $point };`
	stmt, err := aql.Parse([]byte(insertAQL))
	if err != nil {
		t.Fatalf("parse insert error: %v", err)
	}
	compiledInsert, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile insert error: %v", err)
	}
	insertSQL := compiledInsert.SQL
	if !strings.Contains(insertSQL, "ST_SetSRID(ST_MakePoint") {
		t.Errorf("expected insert SQL to contain ST_SetSRID(ST_MakePoint, got:\n%s", insertSQL)
	}
	if !strings.Contains(insertSQL, "->>'longitude'") || !strings.Contains(insertSQL, "->>'latitude'") {
		t.Errorf("expected insert SQL to extract longitude and latitude from jsonb parameter, got:\n%s", insertSQL)
	}

	// 2. Test UPDATE serialization
	updateAQL := `update Location filter .id = $id set { point := $point };`
	stmtUpdate, err := aql.Parse([]byte(updateAQL))
	if err != nil {
		t.Fatalf("parse update error: %v", err)
	}
	compiledUpdate, err := compiler.Compile(stmtUpdate, ir)
	if err != nil {
		t.Fatalf("compile update error: %v", err)
	}
	updateSQL := compiledUpdate.SQL
	if !strings.Contains(updateSQL, "SET\n  point = ST_SetSRID(ST_MakePoint") {
		t.Errorf("expected update SQL to set point via ST_SetSRID(ST_MakePoint, got:\n%s", updateSQL)
	}
	if !strings.Contains(updateSQL, "->>'longitude'") || !strings.Contains(updateSQL, "->>'latitude'") {
		t.Errorf("expected update SQL to extract longitude and latitude from jsonb parameter, got:\n%s", updateSQL)
	}
}

