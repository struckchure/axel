package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
)

// ── ASL ──────────────────────────────────────────────────────────────────────

func TestASLFormatExampleIdempotent(t *testing.T) {
	src, err := os.ReadFile("../examples/basic/default.asl")
	if err != nil {
		t.Fatal(err)
	}
	out, err := asl.Format(src)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	// Formatted output must parse.
	if _, err := asl.Parse([]byte(out)); err != nil {
		t.Fatalf("formatted output does not parse: %v", err)
	}
	// Formatting is idempotent.
	out2, err := asl.Format([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if out != out2 {
		t.Errorf("ASL formatting not idempotent:\n--- first ---\n%s\n--- second ---\n%s", out, out2)
	}
}

func TestASLFormatNormalizesWhitespace(t *testing.T) {
	messy := "type    A{required   id:uuid;name:str;}"
	out, err := asl.Format([]byte(messy))
	if err != nil {
		t.Fatal(err)
	}
	want := "type A {\n  required id: uuid;\n  name: str;\n}\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestASLFormatPreservesComments(t *testing.T) {
	src := "# header\n" +
		"type User {\n" +
		"  required email: str;  # login\n" +
		"  # age note\n" +
		"  required age: int32;\n" +
		"}\n"
	out, err := asl.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# header", "# login", "# age note"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatted output dropped comment %q:\n%s", want, out)
		}
	}
	// The trailing comment stays on the email line; the note stays above age.
	if !strings.Contains(out, "required email: str;  # login") {
		t.Errorf("trailing comment not attached to its line:\n%s", out)
	}
	if !strings.Contains(out, "# age note\n  required age") {
		t.Errorf("leading comment not kept above its field:\n%s", out)
	}
}

// An unparseable schema is a hard error (nothing to format safely).
func TestASLFormatParseErrorReturnsError(t *testing.T) {
	if _, err := asl.Format([]byte("type {")); err == nil {
		t.Error("expected a parse error for malformed schema")
	}
}

// ── AQL ──────────────────────────────────────────────────────────────────────

func TestAQLFormatIdempotent(t *testing.T) {
	queries := []string{
		"select User { id, email } filter .active = true order by .created_at desc limit $n;",
		"multi select Post { id, title, author: { name } } filter .published = true;",
		"insert User { email := $email, name := $name } unless conflict on .email;",
		"update User filter .id = $id set { name := $name, active := true };",
		"delete Session filter .expires_at < $now;",
		"select count(User filter .active = true);",
	}
	for _, q := range queries {
		out, err := aql.Format([]byte(q))
		if err != nil {
			t.Fatalf("format %q: %v", q, err)
		}
		if _, err := aql.Parse([]byte(out)); err != nil {
			t.Fatalf("formatted %q does not parse: %v\n%s", q, err, out)
		}
		out2, err := aql.Format([]byte(out))
		if err != nil {
			t.Fatal(err)
		}
		if out != out2 {
			t.Errorf("AQL formatting not idempotent for %q:\n%s\n---\n%s", q, out, out2)
		}
	}
}

// A qualified enum reference must survive formatting (regression: the printer was
// missing the QualifiedIdent case and would drop it).
func TestAQLFormatKeepsEnumMemberReference(t *testing.T) {
	q := "select Transaction { id } filter .entity = TransactionActorEntity.ApiKey;"
	out, err := aql.Format([]byte(q))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "TransactionActorEntity.ApiKey") {
		t.Errorf("enum member reference dropped by formatter:\n%s", out)
	}
}

func TestAQLFormatPreservesComments(t *testing.T) {
	src := "# list active posts\n" +
		"select Post {\n" +
		"  id,  # pk\n" +
		"  title\n" +
		"} filter .active = true;"
	out, err := aql.Format([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# list active posts", "# pk"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatted query dropped comment %q:\n%s", want, out)
		}
	}
}

func TestAQLFormatParseErrorReturnsError(t *testing.T) {
	if _, err := aql.Format([]byte("select {")); err == nil {
		t.Error("expected a parse error for malformed query")
	}
}
