package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/core/compiler"
	"github.com/struckchure/axel/generators/golang"
)

// typedParamSchema exercises inline param annotations: an enum, a scalar alias,
// an enum-backed column (for inference), and plain scalar columns.
const typedParamSchema = `
enum TransactionStatus { Pending, Settled }
scalar type EmailStr extending str;
type Transaction {
  required id: uuid;
  required status: TransactionStatus;
  required amount: int32;
  required email: EmailStr;
}
`

// paramByName returns the collected param with the given name, or nil.
func paramByName(c *compiler.CompiledSQL, name string) *compiler.ParamInfo {
	for i := range c.Params {
		if c.Params[i].Name == name {
			return &c.Params[i]
		}
	}
	return nil
}

// compileErr parses (must succeed) then compiles, returning the compile error.
func compileErr(t *testing.T, schema, query string) error {
	t.Helper()
	ir := parseSchema(t, schema)
	stmt, err := aql.ParseString(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	_, err = compiler.Compile(stmt, ir)
	return err
}

func TestTypedParamLimitOffset(t *testing.T) {
	c := compileAQL(t, typedParamSchema,
		`multi select Transaction { id } limit $limit<int32>? offset $offset<int32>?;`)

	for _, name := range []string{"limit", "offset"} {
		p := paramByName(c, name)
		if p == nil {
			t.Fatalf("param %q not collected, got %+v", name, c.Params)
		}
		if p.AQLType != "int32" {
			t.Errorf("param %q AQLType = %q, want int32", name, p.AQLType)
		}
		if !p.Optional {
			t.Errorf("param %q should be optional", name)
		}
	}
}

func TestTypedParamEnumAnnotation(t *testing.T) {
	c := compileAQL(t, typedParamSchema,
		`multi select Transaction { id } filter .status = $status<TransactionStatus>;`)

	p := paramByName(c, "status")
	if p == nil {
		t.Fatalf("param status not collected, got %+v", c.Params)
	}
	if p.EnumType != "TransactionStatus" {
		t.Errorf("EnumType = %q, want TransactionStatus", p.EnumType)
	}
}

func TestEnumParamInferenceWithoutAnnotation(t *testing.T) {
	// No annotation — the enum type must still be inferred from the compared column.
	c := compileAQL(t, typedParamSchema,
		`multi select Transaction { id } filter .status = $status;`)

	p := paramByName(c, "status")
	if p == nil {
		t.Fatalf("param status not collected, got %+v", c.Params)
	}
	if p.EnumType != "TransactionStatus" {
		t.Errorf("inferred EnumType = %q, want TransactionStatus", p.EnumType)
	}
}

func TestScalarAliasParamAnnotation(t *testing.T) {
	c := compileAQL(t, typedParamSchema,
		`multi select Transaction { id } filter .email = $email<EmailStr>;`)

	p := paramByName(c, "email")
	if p == nil {
		t.Fatalf("param email not collected, got %+v", c.Params)
	}
	if p.AQLType != "str" {
		t.Errorf("alias param AQLType = %q, want str", p.AQLType)
	}
	if p.EnumType != "" {
		t.Errorf("alias param should have no EnumType, got %q", p.EnumType)
	}
}

func TestObjectTypeParamAnnotationErrors(t *testing.T) {
	err := compileErr(t, typedParamSchema,
		`select Transaction { id } filter .id = $x<Transaction>;`)
	if err == nil {
		t.Fatal("expected error annotating a param with an object type")
	}
	if !strings.Contains(err.Error(), "object type") {
		t.Errorf("error = %v, want mention of object type", err)
	}
}

func TestUnknownParamTypeAnnotationErrors(t *testing.T) {
	err := compileErr(t, typedParamSchema,
		`select Transaction { id } filter .id = $x<Nope>;`)
	if err == nil {
		t.Fatal("expected error for unknown annotation type")
	}
	if !strings.Contains(err.Error(), "unknown parameter type") {
		t.Errorf("error = %v, want 'unknown parameter type'", err)
	}
}

func TestGoCodegenTypedParams(t *testing.T) {
	ir := parseSchema(t, typedParamSchema)
	schema := codegen.FromSchemaIR(ir)

	qPage := buildQueryDesc(t, ir, "listTx", "list_tx.aql",
		`multi select Transaction { id } limit $limit<int32>? offset $offset<int32>?;`)
	qEnum := buildQueryDesc(t, ir, "byStatus", "by_status.aql",
		`multi select Transaction { id } filter .status = $status<TransactionStatus>;`)

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{qPage, qEnum}, &golang.GoGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	// Collapse gofmt's alignment padding so assertions are whitespace-insensitive.
	collapse := func(s string) string { return strings.Join(strings.Fields(s), " ") }

	page := collapse(readFile(t, filepath.Join(dir, "list_tx.go")))
	for _, want := range []string{"Limit *int32", "Offset *int32"} {
		if !strings.Contains(page, want) {
			t.Errorf("list_tx.go missing %q:\n%s", want, page)
		}
	}

	enum := collapse(readFile(t, filepath.Join(dir, "by_status.go")))
	if !strings.Contains(enum, "Status TransactionStatus") {
		t.Errorf("by_status.go missing typed enum param 'Status TransactionStatus':\n%s", enum)
	}
}
