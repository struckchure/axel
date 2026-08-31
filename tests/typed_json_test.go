package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/core/compiler"
	"github.com/struckchure/axel/generators/golang"
	"github.com/struckchure/axel/generators/typescript"
)

const typedJsonSchema = `
scalar type Coordinate extends json {
  lat: str;
  lng: str;
}

scalar type ItemStats extends jsonb {
  score: float64;
  views: int32;
  multi tags: str;
}

type Place {
  required id: uuid;
  name: str;
  coord: Coordinate;
  stats: ItemStats;
  raw_data: json;
}
`

func TestTypedJsonAQLCompilation(t *testing.T) {
	ir := parseSchema(t, typedJsonSchema)

	cases := []struct {
		name       string
		query      string
		wantSQL    []string
		wantParams []compiler.ParamInfo
	}{
		{
			name:  "string field in typed json scalar",
			query: `select Place { id, name } filter .coord.lat = $lat;`,
			wantSQL: []string{
				`FROM "place" p`,
				`(p.coord->>'lat') = $1`,
			},
			wantParams: []compiler.ParamInfo{
				{Name: "lat", AQLType: "str"},
			},
		},
		{
			name:  "float64 field in typed jsonb scalar with cast",
			query: `select Place { id, name } filter .stats.score > $min_score;`,
			wantSQL: []string{
				`FROM "place" p`,
				`((p.stats->>'score')::DOUBLE PRECISION) > $1`,
			},
			wantParams: []compiler.ParamInfo{
				{Name: "min_score", AQLType: "float64"},
			},
		},
		{
			name:  "int32 field in typed jsonb scalar with cast",
			query: `select Place { id, name } filter .stats.views >= 100;`,
			wantSQL: []string{
				`FROM "place" p`,
				`((p.stats->>'views')::INTEGER) >= 100`,
			},
		},
		{
			name:  "untyped json property access",
			query: `select Place { id, name } filter .raw_data.status = 'active';`,
			wantSQL: []string{
				`FROM "place" p`,
				`(p.raw_data->>'status') = 'active'`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt, err := aql.ParseString(tc.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			compiled, err := compiler.Compile(stmt, ir)
			if err != nil {
				t.Fatalf("compile error: %v", err)
			}
			gotSQL := normalizeSQL(compiled.SQL)
			for _, want := range tc.wantSQL {
				if !strings.Contains(gotSQL, normalizeSQL(want)) {
					t.Errorf("SQL missing %q:\n%s", want, compiled.SQL)
				}
			}
			if len(tc.wantParams) > 0 {
				if len(compiled.Params) != len(tc.wantParams) {
					t.Fatalf("params = %+v, want %+v", compiled.Params, tc.wantParams)
				}
				for i, want := range tc.wantParams {
					if got := compiled.Params[i]; got.Name != want.Name || got.AQLType != want.AQLType {
						t.Errorf("param %d = %+v, want %+v", i, got, want)
					}
				}
			}
		})
	}
}

func TestTypedJsonCodegen(t *testing.T) {
	ir := parseSchema(t, typedJsonSchema)
	desc := codegen.FromSchemaIR(ir)
	q := buildQueryDesc(t, ir, "getPlace", "get_place.aql", `select Place { id, name };`)

	// Test TypeScript generator
	tsGen := &typescript.TsGenerator{}
	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir}
	if err := codegen.Walk(desc, []codegen.QueryDescriptor{q}, tsGen, ctx); err != nil {
		t.Fatalf("generate ts: %v", err)
	}

	modelsTs := readFile(t, filepath.Join(dir, "models.ts"))
	if !strings.Contains(modelsTs, "export interface Coordinate {") {
		t.Errorf("missing Coordinate interface in models.ts:\n%s", modelsTs)
	}
	if !strings.Contains(modelsTs, "lat?: string | null;") {
		t.Errorf("missing lat field in Coordinate interface:\n%s", modelsTs)
	}
	if !strings.Contains(modelsTs, "export interface ItemStats {") {
		t.Errorf("missing ItemStats interface in models.ts:\n%s", modelsTs)
	}
	if !strings.Contains(modelsTs, "tags?: string[] | null;") {
		t.Errorf("missing tags field in ItemStats interface:\n%s", modelsTs)
	}
	if !strings.Contains(modelsTs, "coord?: Coordinate | null;") {
		t.Errorf("missing coord property in Place interface:\n%s", modelsTs)
	}
	if !strings.Contains(modelsTs, "stats?: ItemStats | null;") {
		t.Errorf("missing stats property in Place interface:\n%s", modelsTs)
	}

	// Test Go generator
	goGen := &golang.GoGenerator{}
	goDir := t.TempDir()
	goCtx := &codegen.Context{OutDir: goDir}
	if err := codegen.Walk(desc, []codegen.QueryDescriptor{q}, goGen, goCtx); err != nil {
		t.Fatalf("generate go: %v", err)
	}

	modelsGo := readFile(t, filepath.Join(goDir, "models.go"))
	if !strings.Contains(modelsGo, "type Coordinate struct {") {
		t.Errorf("missing Coordinate struct in models.go:\n%s", modelsGo)
	}
	if !strings.Contains(modelsGo, "type ItemStats struct {") {
		t.Errorf("missing ItemStats struct in models.go:\n%s", modelsGo)
	}
	if !strings.Contains(modelsGo, "Tags") || !strings.Contains(modelsGo, "[]string") {
		t.Errorf("missing Tags in ItemStats struct in models.go:\n%s", modelsGo)
	}
	if !strings.Contains(modelsGo, "Coord") || !strings.Contains(modelsGo, "*Coordinate") {
		t.Errorf("missing Coord in Place struct in models.go:\n%s", modelsGo)
	}
}

func TestDDLJsonVsJsonbTypes(t *testing.T) {
	schema := `
scalar type Coordinate extends json {
  lat: str;
  lng: str;
}

scalar type ItemStats extends jsonb {
  score: float64;
  views: int32;
  multi tags: str;
}

type Payload {
  required id: uuid;
  raw_payload: json;
  binary_payload: jsonb;
  coord: Coordinate;
  stats: ItemStats;
}`

	up := genUp(t, schema)

	// Verify exact column types in CREATE TABLE DDL
	for _, want := range []string{
		`"raw_payload" JSON`,
		`"binary_payload" JSONB`,
		`"coord" JSON`,
		`"stats" JSONB`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("expected column definition %q in generated DDL:\n%s", want, up)
		}
	}
}

func TestMultiScalarPropertyDDL(t *testing.T) {
	schema := `
scalar type Route extends jsonb {
  order: int16;
  required latitude: float32;
  required longitude: float32;
  description: str;
  radius: float32;
}

scalar type PackageSize extends jsonb {
  order: int16;
  required length: float32;
  required breadth: float32;
  required height: float32;
  required price: int32;
}

type PackageCondition {
  required id: uuid;
  name: str;
}

type Coverage {
  required id: uuid;
  required name: str;
  multi routes: Route;
  multi sizes: PackageSize;
  multi conditions: PackageCondition;
}
`

	up, down := genMigrationFull(t, "", schema)

	// Coverage table should have "routes" JSONB and "sizes" JSONB columns directly
	if !strings.Contains(up, `"routes" JSONB`) {
		t.Errorf("expected \"routes\" JSONB column in Coverage table:\n%s", up)
	}
	if !strings.Contains(up, `"sizes" JSONB`) {
		t.Errorf("expected \"sizes\" JSONB column in Coverage table:\n%s", up)
	}

	// Should NOT generate coverage_routes or coverage_sizes junction tables
	if strings.Contains(up, "coverage_routes") {
		t.Errorf("did not expect junction table coverage_routes:\n%s", up)
	}
	if strings.Contains(up, "coverage_sizes") {
		t.Errorf("did not expect junction table coverage_sizes:\n%s", up)
	}

	// PackageCondition is an ObjectType, so multi conditions SHOULD generate a junction table
	if !strings.Contains(up, "coverage_conditions") {
		t.Errorf("expected junction table coverage_conditions:\n%s", up)
	}

	// Junction table must be created AFTER both base tables
	ci := strings.Index(up, `CREATE TABLE "coverage"`)
	pci := strings.Index(up, `CREATE TABLE "package_condition"`)
	jci := strings.Index(up, `CREATE TABLE "coverage_conditions"`)
	if ci < 0 || pci < 0 || jci < 0 {
		t.Fatalf("expected coverage, package_condition, and coverage_conditions in up migration:\n%s", up)
	}
	if !(ci < jci && pci < jci) {
		t.Errorf("expected coverage (%d) and package_condition (%d) to precede junction table (%d):\n%s", ci, pci, jci, up)
	}

	// Down should drop coverage_conditions, but not coverage_routes
	if strings.Contains(down, "coverage_routes") {
		t.Errorf("did not expect DROP TABLE coverage_routes:\n%s", down)
	}
	if !strings.Contains(down, "coverage_conditions") {
		t.Errorf("expected DROP TABLE coverage_conditions in down:\n%s", down)
	}
}

func TestCustomSQLExtensionScalarDDL(t *testing.T) {
	schema := `
use extension 'postgis';
use extension 'vector';

scalar type Point extends sql "geography(Point, 4326)" as {
  latitude: float32 := ST_Y(__self__::geometry);
  longitude: float32 := ST_X(__self__::geometry);
};

scalar type Embedding extends sql "vector(1536)" as multi float32;

type Venue {
  required id: uuid { constraint pk; };
  location: Point;
  multi waypoints: Point;
  feature_vec: Embedding;
}
`
	up := genUp(t, schema)

	// Verify exact custom SQL column types in CREATE TABLE DDL
	for _, want := range []string{
		`"location" geography(Point, 4326)`,
		`"waypoints" geography(Point, 4326)[]`,
		`"feature_vec" vector(1536)`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("expected column definition %q in generated DDL:\n%s", want, up)
		}
	}
}



const temporalJsonSchema = `
scalar type VendorAvailability extends jsonb {
  required day: str;
  opening: time;
  closing: time;
  effective_from: date;
  updated_at: datetime;
  multi holidays: date;
}

type Vendor {
  required id: uuid;
  name: str;
  availability: VendorAvailability;
}
`

func TestTypedJsonTemporalFieldsAQLCompilation(t *testing.T) {
	ir := parseSchema(t, temporalJsonSchema)

	cases := []struct {
		name       string
		query      string
		wantSQL    []string
		wantParams []compiler.ParamInfo
	}{
		{
			name:  "time field casts to TIME",
			query: `select Vendor { id, name } filter .availability.opening <= $now;`,
			wantSQL: []string{
				`FROM "vendor" v`,
				`((v.availability->>'opening')::TIME) <= $1`,
			},
			wantParams: []compiler.ParamInfo{
				{Name: "now", AQLType: "time"},
			},
		},
		{
			name:  "date field casts to DATE",
			query: `select Vendor { id, name } filter .availability.effective_from > $from;`,
			wantSQL: []string{
				`((v.availability->>'effective_from')::DATE) > $1`,
			},
			wantParams: []compiler.ParamInfo{
				{Name: "from", AQLType: "date"},
			},
		},
		{
			name:  "datetime field casts to TIMESTAMPTZ",
			query: `select Vendor { id, name } filter .availability.updated_at >= $since;`,
			wantSQL: []string{
				`((v.availability->>'updated_at')::TIMESTAMPTZ) >= $1`,
			},
			wantParams: []compiler.ParamInfo{
				{Name: "since", AQLType: "datetime"},
			},
		},
		{
			name:  "temporal fields in a sub-shape are cast inside json_build_object",
			query: `select Vendor { id, availability: { day, opening, updated_at } };`,
			wantSQL: []string{
				`json_build_object('day', (v.availability->>'day'), 'opening', ((v.availability->>'opening')::TIME), 'updated_at', ((v.availability->>'updated_at')::TIMESTAMPTZ)) AS availability`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt, err := aql.ParseString(tc.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			compiled, err := compiler.Compile(stmt, ir)
			if err != nil {
				t.Fatalf("compile error: %v", err)
			}
			gotSQL := normalizeSQL(compiled.SQL)
			for _, want := range tc.wantSQL {
				if !strings.Contains(gotSQL, normalizeSQL(want)) {
					t.Errorf("SQL missing %q:\n%s", want, compiled.SQL)
				}
			}
			if len(tc.wantParams) > 0 {
				if len(compiled.Params) != len(tc.wantParams) {
					t.Fatalf("params = %+v, want %+v", compiled.Params, tc.wantParams)
				}
				for i, want := range tc.wantParams {
					if got := compiled.Params[i]; got.Name != want.Name || got.AQLType != want.AQLType {
						t.Errorf("param %d = %+v, want %+v", i, got, want)
					}
				}
			}
		})
	}
}

func TestTypedJsonTemporalCodegen(t *testing.T) {
	ir := parseSchema(t, temporalJsonSchema)
	desc := codegen.FromSchemaIR(ir)
	q := buildQueryDesc(t, ir, "getVendor", "get_vendor.aql", `select Vendor { id, name };`)

	tsGen := &typescript.TsGenerator{}
	tsDir := t.TempDir()
	if err := codegen.Walk(desc, []codegen.QueryDescriptor{q}, tsGen, &codegen.Context{OutDir: tsDir}); err != nil {
		t.Fatalf("generate ts: %v", err)
	}
	modelsTs := readFile(t, filepath.Join(tsDir, "models.ts"))
	// Temporal fields inside a JSON document arrive as strings, not Dates — and
	// under the document's own keys, which json_build_object writes in snake_case
	// (`availability->>'effective_from'`), the same names the Go tags carry.
	for _, want := range []string{
		"export interface VendorAvailability {",
		"opening?: string | null;",
		"effective_from?: string | null;",
		"updated_at?: string | null;",
		"holidays?: string[] | null;",
	} {
		if !strings.Contains(modelsTs, want) {
			t.Errorf("models.ts missing %q:\n%s", want, modelsTs)
		}
	}
	if strings.Contains(modelsTs, "Date") {
		t.Errorf("models.ts should not type JSON temporal fields as Date:\n%s", modelsTs)
	}

	goGen := &golang.GoGenerator{}
	goDir := t.TempDir()
	if err := codegen.Walk(desc, []codegen.QueryDescriptor{q}, goGen, &codegen.Context{OutDir: goDir}); err != nil {
		t.Fatalf("generate go: %v", err)
	}
	modelsGo := readFile(t, filepath.Join(goDir, "models.go"))
	for _, want := range []string{
		"type VendorAvailability struct {",
		"Opening *string `json:\"opening\"`",
		"EffectiveFrom *string `json:\"effective_from\"`",
		"UpdatedAt *string `json:\"updated_at\"`",
		"Holidays []string `json:\"holidays\"`",
	} {
		if !strings.Contains(normalizeSQL(modelsGo), normalizeSQL(want)) {
			t.Errorf("models.go missing %q:\n%s", want, modelsGo)
		}
	}
	if strings.Contains(modelsGo, "time.Time") {
		t.Errorf("models.go should not type JSON temporal fields as time.Time:\n%s", modelsGo)
	}
}

func TestTypedJsonDisallowedFieldTypes(t *testing.T) {
	for _, bad := range []string{"bool", "uuid", "bytes", "json"} {
		schema := "scalar type S extends jsonb {\n  f: " + bad + ";\n}\n\ntype T {\n  required id: uuid;\n  s: S;\n}\n"
		err := resolveErr(t, schema)
		if err == nil {
			t.Errorf("type %q: expected a resolve error, got nil", bad)
			continue
		}
		if !strings.Contains(err.Error(), "is not allowed") {
			t.Errorf("type %q: unexpected error: %v", bad, err)
		}
	}
}
