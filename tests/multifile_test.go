package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/asl"
)

// writeSplitSchema writes each named file into a temp dir and returns a glob
// matching them all.
func writeSplitSchema(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "*.asl")
}

func loadSplit(t *testing.T, files map[string]string) *asl.SchemaIR {
	t.Helper()
	ir, _, err := asl.LoadIR(writeSplitSchema(t, files))
	if err != nil {
		t.Fatalf("LoadIR: %v", err)
	}
	return ir
}

func splitErr(t *testing.T, files map[string]string) error {
	t.Helper()
	_, _, err := asl.LoadIR(writeSplitSchema(t, files))
	return err
}

// A link may point at a type declared in another file — the files share one
// flat namespace.
func TestSplitSchemaCrossFileLink(t *testing.T) {
	ir := loadSplit(t, map[string]string{
		"author.asl": "type Author { required name: str; }\n",
		"book.asl":   "type Book { required title: str; required link author: Author; }\n",
	})
	if errs := asl.Validate(ir); len(errs) > 0 {
		t.Fatalf("Validate: %v", errs)
	}
	if got := ir.ObjectTypes["Book"].Links["author"].TargetType; got != "Author" {
		t.Errorf("Book.author target = %q, want Author", got)
	}
}

// The child's file sorts before the parent's, so the parent is merged in second.
// Inheritance must still flatten — this is the regression that makes lexical
// file ordering safe.
func TestSplitSchemaInheritanceIgnoresFileOrder(t *testing.T) {
	ir := loadSplit(t, map[string]string{
		"a_child.asl":  "type Post extending Timestamped { required title: str; }\n",
		"z_parent.asl": "abstract type Timestamped { required created_at: datetime; }\n",
	})
	post, ok := ir.ObjectTypes["Post"]
	if !ok {
		t.Fatal("Post missing")
	}
	if _, ok := post.Properties["created_at"]; !ok {
		t.Errorf("Post did not inherit created_at; properties = %v", keysOf(post.Properties))
	}
}

// The same must hold inside a single file: a child declared above its parent.
func TestInheritanceIgnoresDeclarationOrder(t *testing.T) {
	ir := parseSchema(t, `
		type Post extending Timestamped { required title: str; }
		abstract type Timestamped { required created_at: datetime; }
	`)
	if _, ok := ir.ObjectTypes["Post"].Properties["created_at"]; !ok {
		t.Error("Post did not inherit created_at from a parent declared below it")
	}
}

// A scalar may extend a scalar declared later, or in another file.
func TestSplitSchemaScalarChainIgnoresFileOrder(t *testing.T) {
	ir := loadSplit(t, map[string]string{
		"a_derived.asl": "scalar type Email extending ShortStr;\n",
		"z_base.asl":    "scalar type ShortStr extending str;\n",
	})
	if got := ir.ScalarTypes["Email"].SQLType; got != "TEXT" {
		t.Errorf("Email SQL type = %q, want TEXT", got)
	}
}

// A global declared in one file is usable from a policy in another.
func TestSplitSchemaCrossFileGlobal(t *testing.T) {
	ir := loadSplit(t, map[string]string{
		"a_model.asl":   "type Doc { required owner: uuid; policy owner_only for select using ( .owner = global current_user ); }\n",
		"z_globals.asl": "global required current_user: uuid;\n",
	})
	if len(ir.ObjectTypes["Doc"].Policies) != 1 {
		t.Fatalf("policies = %d, want 1", len(ir.ObjectTypes["Doc"].Policies))
	}
}

// `use extension` repeated per file is expected in a split schema and must be
// deduped rather than rejected.
func TestSplitSchemaDedupesExtensions(t *testing.T) {
	ir := loadSplit(t, map[string]string{
		"a.asl": "use extension 'uuid-ossp';\ntype A { required id: uuid; }\n",
		"b.asl": "use extension 'uuid-ossp';\ntype B { required id: uuid; }\n",
	})
	if len(ir.Extensions) != 1 {
		t.Errorf("extensions = %v, want one entry", ir.Extensions)
	}
}

// A name declared in two files is an error naming both sites — without this a
// whole model is silently dropped.
func TestSplitSchemaDuplicateDeclarations(t *testing.T) {
	cases := map[string]map[string]string{
		"type": {
			"a.asl": "type User { required name: str; }\n",
			"b.asl": "type User { required email: str; }\n",
		},
		"enum": {
			"a.asl": "enum Role { Admin }\n",
			"b.asl": "enum Role { Member }\n",
		},
		"scalar": {
			"a.asl": "scalar type Email extending str;\n",
			"b.asl": "scalar type Email extending str;\n",
		},
		"global": {
			"a.asl": "global tz: str;\n",
			"b.asl": "global tz: str;\n",
		},
		"across kinds": {
			"a.asl": "enum Status { Draft }\n",
			"b.asl": "type Status { required name: str; }\n",
		},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			err := splitErr(t, files)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, f := range []string{"a.asl", "b.asl"} {
				if !strings.Contains(err.Error(), f) {
					t.Errorf("error should name %s: %v", f, err)
				}
			}
		})
	}
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
