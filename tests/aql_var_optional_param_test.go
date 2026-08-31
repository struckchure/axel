package tests

import (
	"strings"
	"testing"
)

const varOptSchema = `type User { required id: uuid; required email: str; required age: int32; }`

// Optionality declared in a var block means the same thing as the inline `$x?`
// form: the comparison is skipped when the value arrives null.
func TestVarBlockOptionalParamSkipsWhenNull(t *testing.T) {
	c := compileAQL(t, varOptSchema,
		"var ( $email: str?; )\nmulti select User { id } filter .email = $email;")

	if !strings.Contains(c.SQL, "($1::TEXT IS NULL OR u.email = $1::TEXT)") {
		t.Errorf("expected skip-when-null wrap:\n%s", c.SQL)
	}
}

// The angle-bracket declaration form behaves identically.
func TestVarBlockOptionalAngleFormSkipsWhenNull(t *testing.T) {
	c := compileAQL(t, varOptSchema,
		"var $age<int32>?;\nmulti select User { id } filter .age >= $age;")

	if !strings.Contains(c.SQL, "($1::INTEGER IS NULL OR u.age >= $1::INTEGER)") {
		t.Errorf("expected skip-when-null wrap:\n%s", c.SQL)
	}
}

// Inside an OR the no-op has to drop the arm out instead of matching everything,
// or one omitted param voids the other arms.
func TestVarBlockOptionalParamInOrContext(t *testing.T) {
	c := compileAQL(t, varOptSchema,
		"var ( $email: str?; )\nmulti select User { id } filter .email = $email or .age = 30;")

	if !strings.Contains(c.SQL, "($1::TEXT IS NOT NULL AND u.email = $1") {
		t.Errorf("expected drop-out-of-OR wrap:\n%s", c.SQL)
	}
}

// A required declaration is still compared unconditionally.
func TestVarBlockRequiredParamNotWrapped(t *testing.T) {
	c := compileAQL(t, varOptSchema,
		"var ( $email: str; )\nmulti select User { id } filter .email = $email;")

	if strings.Contains(c.SQL, "IS NULL OR") {
		t.Errorf("required param should not be wrapped:\n%s", c.SQL)
	}
}

// A declared default already substitutes for a null value through COALESCE.
// Skipping the comparison as well would silently ignore that default.
func TestOptionalParamWithDefaultIsNotSkipped(t *testing.T) {
	c := compileAQL(t, varOptSchema,
		"var ( $age: int32? := 21; )\nmulti select User { id } filter .age >= $age;")

	if !strings.Contains(c.SQL, "COALESCE($1::INTEGER, 21)") {
		t.Errorf("expected the default to be coalesced in:\n%s", c.SQL)
	}
	if strings.Contains(c.SQL, "IS NULL OR") {
		t.Errorf("a defaulted param must not also skip the comparison:\n%s", c.SQL)
	}
}

// An array param declared optional in a var block gets the same treatment, with
// every cast of the placeholder staying the array type.
func TestVarBlockOptionalMultiParam(t *testing.T) {
	c := compileAQL(t, varOptSchema,
		"var ( multi $emails: str?; )\nmulti select User { id } filter .email in $emails;")

	if !strings.Contains(c.SQL, "($1::TEXT[] IS NULL OR u.email = ANY($1::TEXT[]))") {
		t.Errorf("expected an array-typed skip-when-null wrap:\n%s", c.SQL)
	}
}
