package codegen

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

const codegenCoverageSchema = `
scalar type Email extending str;
enum Status { Draft, Published }
global tenant_id: uuid;
type User { required id: uuid; required email: Email; computed label := .email; index on (.email); }
type Post { required id: uuid; status: Status; link author: User; multi reviewers: User; }
`

func codegenSchema(t *testing.T) *asl.SchemaIR {
	t.Helper()
	src, err := asl.Parse([]byte(codegenCoverageSchema))
	if err != nil {
		t.Fatal(err)
	}
	ir, err := (&asl.Resolver{}).Resolve(src)
	if err != nil {
		t.Fatal(err)
	}
	return ir
}

func TestDescriptorsRoundTripAndQueryMetadata(t *testing.T) {
	ir := codegenSchema(t)
	schema := FromSchemaIR(ir)
	if len(schema.Scalars) != 1 || len(schema.Enums) != 1 || len(schema.Globals) != 1 || len(schema.Types) != 2 {
		t.Fatalf("schema descriptor = %+v", schema)
	}
	if schema.Types[0].Name != "Post" || schema.Types[1].Name != "User" {
		t.Errorf("types are not deterministic: %+v", schema.Types)
	}
	if back := ToSchemaIR(schema); back.ObjectTypes["Post"].Links["reviewers"].JunctionTable != "post_reviewers" || back.Globals[0].SQLType != "UUID" {
		t.Errorf("round trip schema = %+v", back)
	}

	stmt, err := aql.ParseString(`@name ListPost
@request ListPostInput
@response PostRow
multi select Post { id, status, author: { email } } filter .status = $status<Status>?;`)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatal(err)
	}
	query, err := BuildQueryDescriptor("", "queries/list_post.aql", stmt, compiled, ir)
	if err != nil {
		t.Fatal(err)
	}
	if query.Name != "ListPost" || query.Operation != "select" || !query.Result.IsMultiple || len(query.Result.Fields) != 3 {
		t.Errorf("query descriptor = %+v", query)
	}
	if len(query.Params) != 1 || query.Params[0].EnumType != "Status" || !query.Params[0].IsOptional {
		t.Errorf("params = %+v", query.Params)
	}
	if got := query.RequestType("DefaultInput"); got != "ListPostInput" {
		t.Errorf("RequestType = %q", got)
	}
	if got := query.ResponseType("DefaultRow"); got != "PostRow" {
		t.Errorf("ResponseType = %q", got)
	}
}

func TestWalkTraversalAndDirectiveFailures(t *testing.T) {
	schema := FromSchemaIR(codegenSchema(t))
	gen := &recordingGenerator{}
	query := QueryDescriptor{Name: "listPost", File: "list_post.aql", Operation: "select", Result: ResultDescriptor{IsMultiple: true}}
	if err := Walk(schema, []QueryDescriptor{query}, gen, &Context{OutDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(gen.events, ",")
	for _, want := range []string{"begin", "scalar:Email", "enum:Status", "type:Post", "link:author", "query:listPost", "end"} {
		if !strings.Contains(got, want) {
			t.Errorf("walk events missing %q: %s", want, got)
		}
	}

	bad := QueryDescriptor{File: "bad.aql", Directives: map[string]string{"rel_load_strategy": "eager"}}
	if err := Walk(schema, []QueryDescriptor{bad}, &recordingGenerator{}, &Context{OutDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "rel_load_strategy") {
		t.Errorf("directive error = %v", err)
	}
	if _, err := Lookup("not-installed"); err == nil {
		t.Error("Lookup unknown generator succeeded")
	}
}

type recordingGenerator struct{ events []string }

func (g *recordingGenerator) Name() string { return "recording" }
func (g *recordingGenerator) BeginSchema(*Context, SchemaDescriptor) error {
	g.events = append(g.events, "begin")
	return nil
}
func (g *recordingGenerator) EndSchema(*Context) error {
	g.events = append(g.events, "end")
	return nil
}
func (g *recordingGenerator) BeginType(_ *Context, t TypeDescriptor) error {
	g.events = append(g.events, "type:"+t.Name)
	return nil
}
func (g *recordingGenerator) EndType(*Context) error {
	g.events = append(g.events, "end-type")
	return nil
}
func (g *recordingGenerator) OnProperty(_ *Context, p PropertyDescriptor) error {
	g.events = append(g.events, "property:"+p.Name)
	return nil
}
func (g *recordingGenerator) OnLink(_ *Context, l LinkDescriptor) error {
	g.events = append(g.events, "link:"+l.Name)
	return nil
}
func (g *recordingGenerator) OnComputed(_ *Context, c ComputedDescriptor) error {
	g.events = append(g.events, "computed:"+c.Name)
	return nil
}
func (g *recordingGenerator) OnIndex(*Context, IndexDescriptor) error {
	g.events = append(g.events, "index")
	return nil
}
func (g *recordingGenerator) OnEnum(_ *Context, e EnumDescriptor) error {
	g.events = append(g.events, "enum:"+e.Name)
	return nil
}
func (g *recordingGenerator) OnScalar(_ *Context, s ScalarDescriptor) error {
	g.events = append(g.events, "scalar:"+s.Name)
	return nil
}
func (g *recordingGenerator) OnQuery(_ *Context, q QueryDescriptor) error {
	g.events = append(g.events, "query:"+q.Name)
	return nil
}

func TestExtendedScalarDescriptorRoundTrip(t *testing.T) {
	src, err := asl.Parse([]byte(`
scalar type Code extends str {
  constraint min_length(6);
  constraint max_length(6);
  default := '000000';
}
`))
	if err != nil {
		t.Fatal(err)
	}
	ir, err := (&asl.Resolver{}).Resolve(src)
	if err != nil {
		t.Fatal(err)
	}
	desc := FromSchemaIR(ir)
	if len(desc.Scalars) != 1 {
		t.Fatalf("Scalars len = %d, want 1", len(desc.Scalars))
	}
	s := desc.Scalars[0]
	if s.Name != "Code" || s.Base != "str" || s.Default != "'000000'" || len(s.Constraints) != 2 {
		t.Fatalf("ScalarDescriptor = %+v", s)
	}
	back := ToSchemaIR(desc)
	bScalar := back.ScalarTypes["Code"]
	if bScalar == nil || bScalar.Default != "'000000'" || len(bScalar.Constraints) != 2 {
		t.Fatalf("Reconstructed scalar = %+v", bScalar)
	}
}

