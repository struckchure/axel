package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/golang"
)

func TestEnumQualifiedDefault(t *testing.T) {
	ir := parseSchema(t, `
enum Role { Admin, Member, Guest }
type User {
  required id: uuid { constraint pk; };
  required role: Role { default := Role.Admin; };
}
`)

	prop := ir.ObjectTypes["User"].Properties["role"]
	if prop.EnumType != "Role" {
		t.Errorf("EnumType = %q, want Role", prop.EnumType)
	}
	if prop.Default != "'Admin'" {
		t.Errorf("Default = %q, want 'Admin'", prop.Default)
	}
	if prop.SQLType != "TEXT" {
		t.Errorf("SQLType = %q, want TEXT", prop.SQLType)
	}
}

func TestEnumQuotedDefault(t *testing.T) {
	ir := parseSchema(t, `
enum Role { Admin, Member }
type User { required role: Role { default := 'Member'; }; }
`)
	if got := ir.ObjectTypes["User"].Properties["role"].Default; got != "'Member'" {
		t.Errorf("Default = %q, want 'Member'", got)
	}
}

func TestEnumInvalidMemberErrors(t *testing.T) {
	err := resolveErr(t, `
enum Role { Admin, Member }
type User { required role: Role { default := Role.Wizard; }; }
`)
	if err == nil {
		t.Fatal("expected error for invalid enum member, got nil")
	}
	if !strings.Contains(err.Error(), "Wizard") {
		t.Errorf("error = %v, want mention of Wizard", err)
	}
}

func TestEnumMismatchedNameErrors(t *testing.T) {
	err := resolveErr(t, `
enum Role { Admin }
enum Status { Active }
type User { required role: Role { default := Status.Active; }; }
`)
	if err == nil {
		t.Fatal("expected error for mismatched enum name, got nil")
	}
}

func TestEnumColumnEmitsCheck(t *testing.T) {
	up := genUp(t, `
enum Role { Admin, Member, Guest }
type User {
  required id: uuid { constraint pk; };
  required role: Role { default := Role.Admin; };
}
`)
	for _, want := range []string{
		`CONSTRAINT "chk_user_role_enum" CHECK ("role" IN ('Admin', 'Member', 'Guest'))`,
		`DEFAULT 'Admin'`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("up SQL missing %q:\n%s", want, up)
		}
	}
}

func TestEnumDescriptorRoundTrip(t *testing.T) {
	ir := parseSchema(t, `
enum Role { Admin, Member }
type User { required id: uuid { constraint pk; }; required role: Role; }
`)

	sd := codegen.FromSchemaIR(ir)

	var found bool
	for _, td := range sd.Types {
		if td.Name != "User" {
			continue
		}
		for _, pd := range td.Properties {
			if pd.Name == "role" {
				found = true
				if pd.EnumType != "Role" {
					t.Errorf("descriptor EnumType = %q, want Role", pd.EnumType)
				}
			}
		}
	}
	if !found {
		t.Fatal("role property not found in descriptor")
	}

	// Round-trip back to IR.
	back := codegen.ToSchemaIR(sd)
	if got := back.ObjectTypes["User"].Properties["role"].EnumType; got != "Role" {
		t.Errorf("round-tripped EnumType = %q, want Role", got)
	}
}

// enumRowSchema mirrors the reporter's shape: an optional and a required enum
// column, plus a linked type whose enum column shows up in a nested sub-select row.
const enumRowSchema = `
enum ApplicationStatus { Shutdown, Building, Running }
enum NetworkProtocol { Tcp }
type Application {
  required id: uuid { constraint pk; };
  required name: str;
  status: ApplicationStatus;
}
type Network {
  required id: uuid { constraint pk; };
  required link application: Application;
  required protocol: NetworkProtocol;
}`

// A SELECT result column that is enum-backed must generate as the enum type, not
// *string/string — including columns pulled in by the `*` splat and columns in a
// nested sub-select row.
func TestGoGeneratorTypedEnumResultField(t *testing.T) {
	ir := parseSchema(t, enumRowSchema)
	schema := codegen.FromSchemaIR(ir)

	q := buildQueryDesc(t, ir, "getApplication", "get_application.aql", `select Application {
      *,
      network := (select Network filter .application = Application.id)
    } filter .id = $id<uuid>;`)

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &golang.GoGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	out := collapseSpaces(readFile(t, filepath.Join(dir, "get_application.go")))
	for _, want := range []string{
		"Status *ApplicationStatus", // optional enum from the splat → pointer
		"Protocol NetworkProtocol",  // required enum in the nested sub-select row
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated row missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Status *string") || strings.Contains(out, "Protocol string") {
		t.Errorf("enum column still typed as string:\n%s", out)
	}
}

// collapseSpaces squeezes runs of spaces/tabs to a single space so assertions on
// generated Go are insensitive to gofmt's column alignment.
func collapseSpaces(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' }), " ")
}

// RETURNING rows share the result-field builder, so an insert/update also gets
// enum-typed result columns.
func TestGoGeneratorTypedEnumReturningField(t *testing.T) {
	ir := parseSchema(t, enumRowSchema)
	schema := codegen.FromSchemaIR(ir)

	q := buildQueryDesc(t, ir, "setStatus", "set_status.aql",
		`update Application filter .id = $id<uuid> set { status := $status<ApplicationStatus>? };`)

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, []codegen.QueryDescriptor{q}, &golang.GoGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	out := collapseSpaces(readFile(t, filepath.Join(dir, "set_status.go")))
	if !strings.Contains(out, "Status *ApplicationStatus") {
		t.Errorf("RETURNING row missing enum-typed Status:\n%s", out)
	}
}

// The result-field descriptor must carry the enum type name (mirrors the param path).
func TestEnumResultFieldDescriptor(t *testing.T) {
	ir := parseSchema(t, enumRowSchema)
	q := buildQueryDesc(t, ir, "getApplication", "get_application.aql",
		`select Application { id, status } filter .id = $id<uuid>;`)

	var found bool
	for _, f := range q.Result.Fields {
		if f.Name == "status" {
			found = true
			if f.EnumType != "ApplicationStatus" {
				t.Errorf("status ResultField EnumType = %q, want ApplicationStatus", f.EnumType)
			}
		}
	}
	if !found {
		t.Fatal("status field not found in result descriptor")
	}
}

func TestGoGeneratorTypedEnumField(t *testing.T) {
	ir := parseSchema(t, `
enum Role { Admin, Member, Guest }
type User {
  required id: uuid { constraint pk; };
  required role: Role;
}
`)
	schema := codegen.FromSchemaIR(ir)

	dir := t.TempDir()
	ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"package": "generated"}}
	if err := codegen.Walk(schema, nil, &golang.GoGenerator{}, ctx); err != nil {
		t.Fatalf("walk: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "models.go"))
	if err != nil {
		t.Fatalf("read models.go: %v", err)
	}
	if !strings.Contains(string(data), "Role Role") {
		t.Errorf("models.go missing typed enum field:\n%s", data)
	}
}
