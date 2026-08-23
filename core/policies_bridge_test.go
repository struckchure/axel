package axel

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/asl"
)

// lowerPolicies parses+resolves an ASL source and lowers its policies to SQL via
// the migration bridge (which is where native-AQL predicates are compiled).
func lowerPolicies(t *testing.T, src string) ([]Policy, error) {
	t.Helper()
	sf, err := asl.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ir, err := (&asl.Resolver{}).Resolve(sf)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return SchemaIRToPolicies(ir)
}

func TestPolicyLowersGlobalAndIsNull(t *testing.T) {
	pols, err := lowerPolicies(t, `
global current_user: uuid;

type Doc {
  required owner: uuid;
  required title: str;
  deleted_at: datetime;

  policy owner_only for all to app_user
    using ( .owner = global current_user )
    with check ( .owner = global current_user );
  policy hide_deleted for select using ( .deleted_at is null );
}`)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if len(pols) != 2 {
		t.Fatalf("want 2 policies, got %d", len(pols))
	}

	byName := map[string]string{}
	for _, p := range pols {
		byName[p.Name] = p.CreateSQL
	}

	owner := byName["doc.owner_only"]
	wantUsing := `USING (owner = current_setting('app.current_user', true)::UUID)`
	if !strings.Contains(owner, wantUsing) {
		t.Errorf("owner_only USING missing %q:\n%s", wantUsing, owner)
	}
	if !strings.Contains(owner, `WITH CHECK (owner = current_setting('app.current_user', true)::UUID)`) {
		t.Errorf("owner_only WITH CHECK wrong:\n%s", owner)
	}
	if !strings.Contains(owner, "TO app_user") {
		t.Errorf("owner_only missing role:\n%s", owner)
	}

	if hide := byName["doc.hide_deleted"]; !strings.Contains(hide, `USING (deleted_at IS NULL)`) {
		t.Errorf("hide_deleted wrong:\n%s", hide)
	}
}

func TestPolicyMultipleCommandsLowerToSeparateStatements(t *testing.T) {
	pols, err := lowerPolicies(t, `
type Event {
  required topic: str;
  policy enforce_append_only for update, delete using ( false );
}`)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if len(pols) != 2 {
		t.Fatalf("want 2 policies (one per command), got %d", len(pols))
	}

	byName := map[string]string{}
	for _, p := range pols {
		byName[p.Name] = p.CreateSQL
	}

	up, ok := byName["event.enforce_append_only_update"]
	if !ok {
		t.Fatalf("missing update policy; got %v", byName)
	}
	if !strings.Contains(up, `CREATE POLICY "enforce_append_only_update" ON "event" FOR UPDATE`) {
		t.Errorf("update policy wrong:\n%s", up)
	}
	if !strings.Contains(up, `USING (false)`) {
		t.Errorf("update policy missing using:\n%s", up)
	}

	del, ok := byName["event.enforce_append_only_delete"]
	if !ok {
		t.Fatalf("missing delete policy; got %v", byName)
	}
	if !strings.Contains(del, `CREATE POLICY "enforce_append_only_delete" ON "event" FOR DELETE`) {
		t.Errorf("delete policy wrong:\n%s", del)
	}
	if !strings.Contains(del, `USING (false)`) {
		t.Errorf("delete policy missing using:\n%s", del)
	}
}

func TestPolicyRequiredGlobalFailsClosed(t *testing.T) {
	pols, err := lowerPolicies(t, `
global required tenant: uuid;
type Row {
  required tenant: uuid;
  policy tenant_iso for all using ( .tenant = global tenant );
}`)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	// A required global reads with missing_ok=false → errors when the GUC is unset.
	want := `current_setting('app.tenant', false)::UUID`
	if !strings.Contains(pols[0].CreateSQL, want) {
		t.Errorf("want %q in:\n%s", want, pols[0].CreateSQL)
	}
}

func TestPolicyLowersToOneLink(t *testing.T) {
	pols, err := lowerPolicies(t, `
global current_user: uuid;

type User { required email: str; }
type Organization { link owner: User; }
type Workflow {
  required name: str;
  link organization: Organization;

  policy owner_only for all to app_user
    using ( .organization.owner = global current_user );
}`)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}

	var using string
	for _, p := range pols {
		if p.Name == "workflow.owner_only" {
			using = p.CreateSQL
		}
	}
	if using == "" {
		t.Fatalf("owner_only policy not found; got %d policies", len(pols))
	}

	// The to-one chain lowers to a correlated subquery over the organization table,
	// correlated to the workflow row's FK by table name.
	for _, want := range []string{
		`SELECT o.owner FROM "organization" o`,
		`WHERE o.id = "workflow".organization`,
		`= current_setting('app.current_user', true)::UUID`,
	} {
		if !strings.Contains(using, want) {
			t.Errorf("owner_only USING missing %q:\n%s", want, using)
		}
	}
}

func TestPolicyLowersMembership(t *testing.T) {
	pols, err := lowerPolicies(t, `
global current_user: uuid;

type User { required email: str; }
type Organization {
  required name: str;
  multi members: User;

  policy member_can_read for select to app_user
    using ( global current_user in .members );
}`)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}

	var using string
	for _, p := range pols {
		if p.Name == "organization.member_can_read" {
			using = p.CreateSQL
		}
	}
	if using == "" {
		t.Fatalf("member_can_read policy not found; got %d policies", len(pols))
	}

	// `global current_user in .members` lowers to an EXISTS-subquery over the junction
	// (organization_members), correlated to the organization row by table name.
	for _, want := range []string{
		`EXISTS (SELECT 1 FROM "organization_members" jt`,
		`WHERE jt.organization = "organization".id`,
		`AND jt.user IN (current_setting('app.current_user', true)::UUID)`,
	} {
		if !strings.Contains(using, want) {
			t.Errorf("member_can_read USING missing %q:\n%s", want, using)
		}
	}
}

func TestPolicyBridgeErrors(t *testing.T) {
	cases := map[string]string{
		"unknown field":  `type T { required a: str; policy p for select using ( .missing = 1 ); }`,
		"unknown global": `type T { required a: str; policy p for select using ( .a = global nope ); }`,
		"bind param":     `type T { required a: str; policy p for select using ( .a = $x ); }`,
		"multi-link scalar traversal": `
type U { required name: str; }
type T { required a: str; multi members: U; policy p for select using ( .members.name = 'x' ); }`,
	}
	for name, src := range cases {
		if _, err := lowerPolicies(t, src); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
