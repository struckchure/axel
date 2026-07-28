package asl

import "testing"

func TestGlobalDeclResolves(t *testing.T) {
	ir := resolveSrc(t, `
global current_user: uuid;
global required tz: str;

type Doc { required title: str; }`)

	if len(ir.Globals) != 2 {
		t.Fatalf("want 2 globals, got %d: %+v", len(ir.Globals), ir.Globals)
	}

	cu := ir.Globals[0]
	if cu.Name != "current_user" || cu.AQLType != "uuid" || cu.SQLType != "UUID" || cu.Required {
		t.Errorf("current_user = %+v", cu)
	}

	tz := ir.Globals[1]
	if tz.Name != "tz" || tz.AQLType != "str" || tz.SQLType != "TEXT" || !tz.Required {
		t.Errorf("tz = %+v", tz)
	}
}

func TestGlobalDeclErrors(t *testing.T) {
	cases := map[string]string{
		"duplicate":    `global a: uuid; global a: str;`,
		"unknown type": `global a: nonsense;`,
		"object type":  `type Doc { required title: str; } global d: Doc;`,
	}
	for name, src := range cases {
		sf, err := Parse([]byte(src))
		if err != nil {
			continue // parse-level rejection is also acceptable
		}
		if _, err := (&Resolver{}).Resolve(sf); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
