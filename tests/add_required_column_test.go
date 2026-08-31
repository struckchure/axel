package tests

import (
	"strings"
	"testing"
)

// `ALTER TABLE ... ADD COLUMN x TEXT NOT NULL` fails on a table that already has
// rows ("column contains null values"), so a required column with no default is
// added nullable and raised to NOT NULL in a follow-up statement, with the
// backfill spelled out in between.
func TestAddRequiredColumnDefersNotNull(t *testing.T) {
	up, _ := genMigration(t,
		`type Vendor { required id: uuid { constraint pk; }; required name: str; }`,
		`type Vendor { required id: uuid { constraint pk; }; required name: str; required description: str; }`)

	if !strings.Contains(up, `ALTER TABLE "vendor" ADD COLUMN "description" TEXT;`) {
		t.Errorf("expected the column to be added nullable first:\n%s", up)
	}
	if !strings.Contains(up, `ALTER TABLE "vendor" ALTER COLUMN "description" SET NOT NULL;`) {
		t.Errorf("expected a separate SET NOT NULL:\n%s", up)
	}
	if !strings.Contains(up, `--   UPDATE "vendor" SET "description" = <value> WHERE "description" IS NULL;`) {
		t.Errorf("expected a backfill seam in the migration:\n%s", up)
	}
	if strings.Contains(up, `ADD COLUMN "description" TEXT NOT NULL`) {
		t.Errorf("required column must not be added NOT NULL inline:\n%s", up)
	}
	// The add must precede the constraint.
	if strings.Index(up, `ADD COLUMN "description"`) > strings.Index(up, `ALTER COLUMN "description" SET NOT NULL`) {
		t.Errorf("SET NOT NULL emitted before the column exists:\n%s", up)
	}
}

// A default backfills existing rows, so NOT NULL stays inline — no second
// statement, no manual backfill.
func TestAddRequiredColumnWithDefaultStaysInline(t *testing.T) {
	up, _ := genMigration(t,
		`type Vendor { required id: uuid { constraint pk; }; }`,
		`type Vendor { required id: uuid { constraint pk; }; required verified: bool { default := false; }; }`)

	if !strings.Contains(up, `ADD COLUMN "verified" BOOLEAN NOT NULL DEFAULT false`) {
		t.Errorf("expected inline NOT NULL DEFAULT:\n%s", up)
	}
	if strings.Contains(up, `ALTER COLUMN "verified" SET NOT NULL`) {
		t.Errorf("a defaulted column needs no follow-up SET NOT NULL:\n%s", up)
	}
}

// An optional column is unaffected.
func TestAddOptionalColumnUnchanged(t *testing.T) {
	up, _ := genMigration(t,
		`type Vendor { required id: uuid { constraint pk; }; }`,
		`type Vendor { required id: uuid { constraint pk; }; description: str; }`)

	if !strings.Contains(up, `ALTER TABLE "vendor" ADD COLUMN "description" TEXT;`) {
		t.Errorf("expected a plain nullable ADD COLUMN:\n%s", up)
	}
	if strings.Contains(up, "SET NOT NULL") {
		t.Errorf("optional column must not gain NOT NULL:\n%s", up)
	}
}

// A required single link has no default to backfill with either, so it gets the
// same treatment — after its FK constraint is in place.
func TestAddRequiredLinkDefersNotNull(t *testing.T) {
	up, _ := genMigration(t,
		`type Vendor { required id: uuid { constraint pk; }; }
type Product { required id: uuid { constraint pk; }; }`,
		`type Vendor { required id: uuid { constraint pk; }; }
type Product { required id: uuid { constraint pk; }; required link vendor: Vendor; }`)

	if !strings.Contains(up, `ALTER TABLE "product" ADD COLUMN "vendor" UUID;`) {
		t.Errorf("expected the FK column to be added nullable first:\n%s", up)
	}
	if !strings.Contains(up, `ALTER TABLE "product" ALTER COLUMN "vendor" SET NOT NULL;`) {
		t.Errorf("expected a separate SET NOT NULL for the link:\n%s", up)
	}
	if !strings.Contains(up, `CONSTRAINT "fk_product_vendor" FOREIGN KEY ("vendor")`) {
		t.Errorf("expected the FK constraint to survive:\n%s", up)
	}
}

// Flipping an existing optional column to required has the same hazard as
// adding one: SET NOT NULL fails while any row still holds a NULL.
func TestModifyToRequiredEmitsBackfillSeam(t *testing.T) {
	up, down := genMigration(t,
		`type Vendor { required id: uuid { constraint pk; }; description: str; }`,
		`type Vendor { required id: uuid { constraint pk; }; required description: str; }`)

	if !strings.Contains(up, `--   UPDATE "vendor" SET "description" = <value> WHERE "description" IS NULL;`) {
		t.Errorf("expected a backfill seam before SET NOT NULL:\n%s", up)
	}
	if !strings.Contains(up, `ALTER TABLE "vendor" ALTER COLUMN "description" SET NOT NULL;`) {
		t.Errorf("expected SET NOT NULL:\n%s", up)
	}
	if strings.Index(up, "UPDATE") > strings.Index(up, "SET NOT NULL") {
		t.Errorf("backfill must precede SET NOT NULL:\n%s", up)
	}
	if !strings.Contains(down, `ALTER TABLE "vendor" ALTER COLUMN "description" DROP NOT NULL;`) {
		t.Errorf("down should drop the constraint:\n%s", down)
	}
}

// With a default declared, the backfill value is known, so the migration fills
// existing rows itself — SET DEFAULT alone would leave them NULL.
func TestModifyToRequiredWithDefaultBackfills(t *testing.T) {
	up, _ := genMigration(t,
		`type Vendor { required id: uuid { constraint pk; }; verified: bool; }`,
		`type Vendor { required id: uuid { constraint pk; }; required verified: bool { default := false; }; }`)

	if !strings.Contains(up, `UPDATE "vendor" SET "verified" = false WHERE "verified" IS NULL;`) {
		t.Errorf("expected an automatic backfill from the default:\n%s", up)
	}
	if strings.Contains(up, "<value>") {
		t.Errorf("a declared default needs no manual backfill:\n%s", up)
	}
	if strings.Index(up, "UPDATE") > strings.Index(up, "SET NOT NULL") {
		t.Errorf("backfill must precede SET NOT NULL:\n%s", up)
	}
}

// Dropping the requirement is unaffected.
func TestModifyToOptionalUnchanged(t *testing.T) {
	up, _ := genMigration(t,
		`type Vendor { required id: uuid { constraint pk; }; required description: str; }`,
		`type Vendor { required id: uuid { constraint pk; }; description: str; }`)

	if !strings.Contains(up, `ALTER TABLE "vendor" ALTER COLUMN "description" DROP NOT NULL;`) {
		t.Errorf("expected DROP NOT NULL:\n%s", up)
	}
	if strings.Contains(up, "UPDATE") {
		t.Errorf("relaxing a column needs no backfill:\n%s", up)
	}
}
