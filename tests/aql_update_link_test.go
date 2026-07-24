package tests

import (
	"strings"
	"testing"
)

const updateLinkSchema = `
type User { required id: uuid; required email: str; }
type GithubInstallation { required id: uuid; required installation_id: int64; }
type Application {
  required id: uuid;
  required name: str;
  link installation: GithubInstallation;
  required link owner: User;
  multi link members: User;
}
`

// A single-link may be assigned in an update, setting its FK column from a
// subquery that resolves to the target's id.
func TestUpdateSingleLinkFromSubquery(t *testing.T) {
	c := compileAQL(t, updateLinkSchema, `update Application filter .id = $id<uuid> set {
	  installation := (select GithubInstallation filter .installation_id = $iid<int64>)
	};`)

	if !strings.Contains(c.SQL, `installation = (SELECT g.id FROM "github_installation" g WHERE g.installation_id = $1 LIMIT 1)`) {
		t.Errorf("link update should set the FK from a subquery:\n%s", c.SQL)
	}
}

// `(select ...) ?? .link` keeps the current FK when the subquery finds nothing —
// the `.installation` fallback resolves to the current row's FK column. The
// optional lookup param uses the IS NOT NULL guard (value-filter identity): when
// $iid is omitted the subquery must yield NULL so the fallback fires, not match
// every installation and return an arbitrary one.
func TestUpdateLinkCoalesceKeepsCurrent(t *testing.T) {
	c := compileAQL(t, updateLinkSchema, `update Application filter .id = $id<uuid> set {
	  installation := (select GithubInstallation filter .installation_id = $iid<int64>?) ?? .installation
	};`)

	want := `installation = COALESCE((SELECT g.id FROM "github_installation" g WHERE ($1::BIGINT IS NOT NULL AND g.installation_id = $1) LIMIT 1), a.installation)`
	if !strings.Contains(c.SQL, want) {
		t.Errorf("expected coalesce-to-current-FK, want:\n%s\ngot:\n%s", want, c.SQL)
	}
}

// A single-link may also be set directly from a uuid param (the FK value).
func TestUpdateLinkFromParam(t *testing.T) {
	c := compileAQL(t, updateLinkSchema, `update Application filter .id = $id<uuid> set {
	  owner := $owner
	};`)
	if !strings.Contains(c.SQL, "owner = $1") {
		t.Errorf("link should be assignable from a param:\n%s", c.SQL)
	}
	if p := paramByName(c, "owner"); p == nil || p.AQLType != "uuid" {
		t.Errorf("a bare FK param should infer uuid, got %+v", p)
	}
}

// Multi-link assignment in an update is rejected with a clear error.
func TestUpdateMultiLinkRejected(t *testing.T) {
	err := compileErr(t, updateLinkSchema, `update Application filter .id = $id<uuid> set {
	  members := (select User filter .id = $u<uuid>)
	};`)
	if err == nil || !strings.Contains(err.Error(), "multi-link") {
		t.Errorf("expected a multi-link-not-supported error, got %v", err)
	}
}

// Assigning a field that is neither a property nor a link still errors.
func TestUpdateUnknownFieldRejected(t *testing.T) {
	err := compileErr(t, updateLinkSchema, `update Application filter .id = $id<uuid> set { nope := $x };`)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("expected an unknown-field error, got %v", err)
	}
}
