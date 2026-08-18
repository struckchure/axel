package asl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree creates files (relative path → contents) under a fresh temp dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestExpandPaths(t *testing.T) {
	root := writeTree(t, map[string]string{
		"schema/a.asl":            "",
		"schema/b.asl":            "",
		"schema/nested/c.asl":     "",
		"schema/notes.md":         "",
		"schema/.hidden/skip.asl": "",
		"other.asl":               "",
	})

	rel := func(paths []string) []string {
		out := make([]string, len(paths))
		for i, p := range paths {
			r, _ := filepath.Rel(root, p)
			out[i] = filepath.ToSlash(r)
		}
		return out
	}

	cases := []struct {
		name string
		spec string
		want []string
	}{
		{"single file", "other.asl", []string{"other.asl"}},
		{"directory walks recursively", "schema", []string{"schema/a.asl", "schema/b.asl", "schema/nested/c.asl"}},
		{"glob is one level", "schema/*.asl", []string{"schema/a.asl", "schema/b.asl"}},
		{"doublestar is recursive", "schema/**/*.asl", []string{"schema/a.asl", "schema/b.asl", "schema/nested/c.asl"}},
		{"doublestar with a name", "schema/**/c.asl", []string{"schema/nested/c.asl"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandPaths(filepath.Join(root, filepath.FromSlash(tc.spec)))
			if err != nil {
				t.Fatalf("ExpandPaths(%q): %v", tc.spec, err)
			}
			if strings.Join(rel(got), ",") != strings.Join(tc.want, ",") {
				t.Errorf("ExpandPaths(%q) = %v, want %v", tc.spec, rel(got), tc.want)
			}
		})
	}
}

func TestExpandPathsErrors(t *testing.T) {
	root := writeTree(t, map[string]string{"schema/a.asl": ""})

	t.Run("empty spec", func(t *testing.T) {
		if _, err := ExpandPaths(""); err == nil {
			t.Error("expected an error")
		}
	})
	t.Run("missing file", func(t *testing.T) {
		if _, err := ExpandPaths(filepath.Join(root, "nope.asl")); err == nil {
			t.Error("expected an error")
		}
	})
	// A glob matching nothing must fail loudly: an empty schema would make the
	// migration diff look like "drop everything".
	t.Run("glob matches nothing", func(t *testing.T) {
		_, err := ExpandPaths(filepath.Join(root, "queries", "*.asl"))
		if err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("directory with no .asl", func(t *testing.T) {
		empty := t.TempDir()
		if _, err := ExpandPaths(empty); err == nil {
			t.Error("expected an error")
		}
	})
}

func TestLoadMergesFilesAndNamesPositions(t *testing.T) {
	root := writeTree(t, map[string]string{
		"schema/a.asl": "type Author { required name: str; }\n",
		"schema/b.asl": "type Book { required title: str; }\n",
	})

	ir, files, err := LoadIR(filepath.Join(root, "schema", "*.asl"))
	if err != nil {
		t.Fatalf("LoadIR: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %v, want 2", files)
	}
	for _, name := range []string{"Author", "Book"} {
		if _, ok := ir.ObjectTypes[name]; !ok {
			t.Errorf("type %q missing from merged schema", name)
		}
	}

	// Positions must carry the file they came from, so errors can name it.
	sf, _, err := Load(filepath.Join(root, "schema", "b.asl"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sf.Definitions[0].TypeDef.Pos.Filename; !strings.HasSuffix(got, "b.asl") {
		t.Errorf("Pos.Filename = %q, want it to end in b.asl", got)
	}
}
