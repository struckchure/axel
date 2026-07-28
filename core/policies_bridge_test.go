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

func TestPolicyBridgeErrors(t *testing.T) {
	cases := map[string]string{
		"unknown field":  `type T { required a: str; policy p for select using ( .missing = 1 ); }`,
		"unknown global": `type T { required a: str; policy p for select using ( .a = global nope ); }`,
		"bind param":     `type T { required a: str; policy p for select using ( .a = $x ); }`,
		"link traversal": `
type U { required name: str; }
type T { required a: str; link owner: U; policy p for select using ( .owner.name = 'x' ); }`,
	}
	for name, src := range cases {
		if _, err := lowerPolicies(t, src); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
