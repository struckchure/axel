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
		`CHECK ("role" IN ('Admin', 'Member', 'Guest'))`,
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
