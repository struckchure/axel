package tests

import (
	"strings"
	"testing"
)

const inferSchema = `
type User { required id: uuid; required email: str; }
type Organization { required id: uuid; required owner: User; }
type Project { required id: uuid; required organization: Organization; }
type GithubInstallation { required id: uuid; required installation_id: int64; }
type Application {
  required id: uuid;
  name: str;
  project: Project;
  installation: GithubInstallation;
}
`

// A computed path field's type is inferred by resolving the path — no cast
// needed — and produces no warning.
func TestComputedPathTypeInferred(t *testing.T) {
	ir := parseSchema(t, inferSchema)
	desc := buildQueryDesc(t, ir, "Q", "q.aql", `multi select Application {
	  id,
	  owner := .project.organization.owner.id,
	  iid := .installation.installation_id
	} filter .id = $id<uuid>;`)

	types := map[string]string{}
	for _, f := range desc.Result.Fields {
		types[f.Name] = f.AQLType
	}
	if types["owner"] != "uuid" {
		t.Errorf("owner should infer uuid, got %q", types["owner"])
	}
	if types["iid"] != "int64" {
		t.Errorf("iid should infer int64, got %q", types["iid"])
	}
	if len(desc.Warnings) != 0 {
		t.Errorf("inferable paths should produce no warnings, got %v", desc.Warnings)
	}
}

// A path ending on a link infers uuid (the FK reference column).
func TestComputedPathToLinkInfersUUID(t *testing.T) {
	ir := parseSchema(t, inferSchema)
	desc := buildQueryDesc(t, ir, "Q", "q.aql", `multi select Application {
	  id,
	  org := .project.organization
	} filter .id = $id<uuid>;`)
	for _, f := range desc.Result.Fields {
		if f.Name == "org" && f.AQLType != "uuid" {
			t.Errorf("path ending on a link should infer uuid, got %q", f.AQLType)
		}
	}
}

// A computed expression that isn't a simple path can't be inferred: it warns and
// falls back to json (`any`).
func TestComputedNonPathWarnsAndFallsBack(t *testing.T) {
	ir := parseSchema(t, inferSchema)
	desc := buildQueryDesc(t, ir, "Q", "q.aql", `multi select Application {
	  id,
	  who := .name ?? .id
	} filter .id = $id<uuid>;`)

	if len(desc.Warnings) != 1 || !strings.Contains(desc.Warnings[0], "who") {
		t.Fatalf("expected one warning naming `who`, got %v", desc.Warnings)
	}
	if !strings.Contains(desc.Warnings[0], "cast") {
		t.Errorf("warning should suggest adding a cast, got %q", desc.Warnings[0])
	}
	for _, f := range desc.Result.Fields {
		if f.Name == "who" && f.AQLType != "json" {
			t.Errorf("uninferable computed field should fall back to json, got %q", f.AQLType)
		}
	}
}

// A cast on a parenthesized expression types an otherwise un-inferable field
// (e.g. a coalesce) and silences the warning.
func TestParenExprCastTypesComputedField(t *testing.T) {
	ir := parseSchema(t, inferSchema)
	desc := buildQueryDesc(t, ir, "Q", "q.aql", `multi select Application {
	  id,
	  who := (.name ?? .id)<str>
	} filter .id = $id<uuid>;`)

	for _, f := range desc.Result.Fields {
		if f.Name == "who" && f.AQLType != "str" {
			t.Errorf("(expr)<str> should type the field str, got %q", f.AQLType)
		}
	}
	if len(desc.Warnings) != 0 {
		t.Errorf("a cast expression should produce no warning, got %v", desc.Warnings)
	}

	// The cast reaches the SQL as ::TEXT.
	c := compileAQL(t, inferSchema, `multi select Application { id, who := (.name ?? .id)<str> } filter .id = $id<uuid>;`)
	if !strings.Contains(c.SQL, "::TEXT) AS who") {
		t.Errorf("expression cast should emit ::TEXT:\n%s", c.SQL)
	}
}

// An explicit cast overrides inference (and silences any warning).
func TestComputedCastOverridesInference(t *testing.T) {
	ir := parseSchema(t, inferSchema)
	desc := buildQueryDesc(t, ir, "Q", "q.aql", `multi select Application {
	  id,
	  owner := .project.organization.owner.id<str>
	} filter .id = $id<uuid>;`)
	for _, f := range desc.Result.Fields {
		if f.Name == "owner" && f.AQLType != "str" {
			t.Errorf("explicit cast should win over inference (uuid), got %q", f.AQLType)
		}
	}
	if len(desc.Warnings) != 0 {
		t.Errorf("a cast should produce no warning, got %v", desc.Warnings)
	}
}
