package tests

import (
	"strings"
	"testing"
)

const multiScalarSchema = `
enum UserType { Admin, Runner, Vendor }

type User {
  required id: uuid { constraint pk };
  required email: str;
  multi roles: UserType;
  multi images: str;
}

type Post {
  required id: uuid { constraint pk };
  required title: str;
  required link author: User;
}

type Vendor {
  required id: uuid { constraint pk };
  required name: str;
  multi link members: User;
}`

// A `multi` scalar is an array column (TEXT[]), so membership against it must
// lower to `= ANY(...)`. Postgres `IN` takes a parenthesised list, not an array,
// so the old verbatim `x in u.roles` compiled cleanly and then failed at runtime.
func TestInOnMultiScalarUsesAny(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{
			"enum member",
			`multi select User { id } filter UserType.Admin in .roles;`,
			`'Admin' = ANY(u.roles)`,
		},
		{
			"string literal against a multi str",
			`multi select User { id } filter 'avatar.png' in .images;`,
			`'avatar.png' = ANY(u.images)`,
		},
		{
			"bind param",
			`multi select User { id } filter $role in .roles;`,
			`$1 = ANY(u.roles)`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compileAQL(t, multiScalarSchema, tc.query).SQL
			if !strings.Contains(got, tc.want) {
				t.Errorf("SQL missing %q:\n%s", tc.want, got)
			}
			if strings.Contains(got, " in u.") {
				t.Errorf("SQL still emits a verbatim `in` against an array column:\n%s", got)
			}
		})
	}
}

// An optional param keeps its null guard around the `= ANY`, and the guard's
// cast uses the array's *element* type — that is what ANY compares against.
func TestInOnMultiScalarWithOptionalParam(t *testing.T) {
	got := compileAQL(t, multiScalarSchema, `multi select User { id } filter $role? in .roles;`).SQL
	for _, want := range []string{"$1::TEXT IS NULL", "$1 = ANY(u.roles)"} {
		if !strings.Contains(got, want) {
			t.Errorf("SQL missing %q:\n%s", want, got)
		}
	}
}

// A single-link prefix locates the row that owns the array, so the array itself
// is resolved through a correlated subquery. That subquery has to be cast:
// `ANY (<subquery>)` is the *subquery* form of ANY, which Postgres reads as a set
// of rows and then rejects with `operator does not exist: text = text[]`. The
// cast makes it an ordinary array expression and selects the array form.
func TestInOnMultiScalarThroughSingleLink(t *testing.T) {
	got := compileAQL(t, multiScalarSchema, `multi select Post { id } filter UserType.Admin in .author.roles;`).SQL
	if !strings.Contains(got, "= ANY((SELECT") {
		t.Errorf("expected ANY over a correlated subquery:\n%s", got)
	}
	if !strings.Contains(got, ")::TEXT[])") {
		t.Errorf("subquery operand must be cast to the array type:\n%s", got)
	}
}

// Regression: a multi *link* is a junction membership test and must keep its
// EXISTS lowering — compileMembership still runs before the ANY branch.
func TestInOnMultiLinkStillUsesExists(t *testing.T) {
	got := compileAQL(t, multiScalarSchema, `multi select Vendor { id } filter $u<uuid> in .members;`).SQL
	if !strings.Contains(got, `EXISTS (SELECT 1 FROM "vendor_members"`) {
		t.Errorf("expected EXISTS over the junction table:\n%s", got)
	}
	if strings.Contains(got, "ANY(") {
		t.Errorf("multi link should not compile to ANY:\n%s", got)
	}
}

// Regression: `in` against a plain scalar is untouched.
func TestInOnSingleScalarUnchanged(t *testing.T) {
	got := compileAQL(t, multiScalarSchema, `multi select User { id } filter $e in .email;`).SQL
	if strings.Contains(got, "ANY(") {
		t.Errorf("single scalar should not compile to ANY:\n%s", got)
	}
}
