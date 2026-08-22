package tests

import (
	"strings"
	"testing"
)

const multiLinkMutationSchema = `
type User {
  required id: uuid;
  required email: str;
  department: str;
  active: bool;
}

type Tag {
  required id: uuid;
  required name: str;
}

type Application {
  required id: uuid;
  required name: str;
  link owner: User;
  multi link members: User;
  multi link tags: Tag;
}
`

func TestUpdateMultiLinkDeltaAdd(t *testing.T) {
	c := compileAQL(t, multiLinkMutationSchema, `
update Application filter .id = $id<uuid> set {
  members := {
    "+": (multi select User filter .email in $emails)
  }
};`)

	if !strings.Contains(c.SQL, `_target AS (`) || !strings.Contains(c.SQL, `_ins_members AS (`) {
		t.Fatalf("expected CTE pipeline with _target and _ins_members:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `INSERT INTO "application_members" ("application", "user")`) {
		t.Errorf("expected insert into junction table application_members:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `ON CONFLICT DO NOTHING`) {
		t.Errorf("expected ON CONFLICT DO NOTHING:\n%s", c.SQL)
	}
}

func TestUpdateMultiLinkDeltaRemove(t *testing.T) {
	c := compileAQL(t, multiLinkMutationSchema, `
update Application filter .id = $id<uuid> set {
  members := {
    "-": (select User filter .id = $user_id<uuid>)
  }
};`)

	if !strings.Contains(c.SQL, `_target AS (`) || !strings.Contains(c.SQL, `_del_members AS (`) {
		t.Fatalf("expected CTE pipeline with _target and _del_members:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `DELETE FROM "application_members"`) {
		t.Errorf("expected delete from junction table application_members:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `WHERE "application" IN (SELECT id FROM _target)`) {
		t.Errorf("expected application in target:\n%s", c.SQL)
	}
}

func TestUpdateMultiLinkDeltaBothAddAndRemove(t *testing.T) {
	c := compileAQL(t, multiLinkMutationSchema, `
update Application filter .id = $id<uuid> set {
  name := $new_name<str>,
  members := {
    "+": (multi select User filter .department = 'Engineering'),
    "-": (select User filter .id = $old_user<uuid>)
  }
};`)

	if !strings.Contains(c.SQL, `UPDATE "application" a SET`) || !strings.Contains(c.SQL, `name = $1`) {
		t.Errorf("expected target update on name:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `_del_members AS (`) || !strings.Contains(c.SQL, `_ins_members AS (`) {
		t.Errorf("expected both _del_members and _ins_members CTEs:\n%s", c.SQL)
	}
	// Verify del occurs before ins
	delIdx := strings.Index(c.SQL, `_del_members AS`)
	insIdx := strings.Index(c.SQL, `_ins_members AS`)
	if !(delIdx < insIdx) {
		t.Errorf("expected delete CTE to precede insert CTE:\n%s", c.SQL)
	}
}

func TestUpdateMultiLinkFullReplacement(t *testing.T) {
	c := compileAQL(t, multiLinkMutationSchema, `
update Application filter .id = $id<uuid> set {
  members := (multi select User filter .active = true)
};`)

	if !strings.Contains(c.SQL, `_del_members AS (`) || !strings.Contains(c.SQL, `_ins_members AS (`) {
		t.Fatalf("expected delete-all and insert CTEs for full replacement:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `DELETE FROM "application_members"`) || !strings.Contains(c.SQL, `INSERT INTO "application_members"`) {
		t.Errorf("expected full replace delete and insert:\n%s", c.SQL)
	}
}

func TestUpdateMultipleMultiLinksInOneStatement(t *testing.T) {
	c := compileAQL(t, multiLinkMutationSchema, `
update Application filter .id = $id<uuid> set {
  members := { "+": (select User filter .id = $u<uuid>) },
  tags := { "+": (select Tag filter .name = 'backend'), "-": (select Tag filter .name = 'deprecated') }
};`)

	if !strings.Contains(c.SQL, `_ins_members AS (`) {
		t.Errorf("expected _ins_members CTE:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `_del_tags AS (`) || !strings.Contains(c.SQL, `_ins_tags AS (`) {
		t.Errorf("expected _del_tags and _ins_tags CTEs:\n%s", c.SQL)
	}
}

func TestUpdateMultiLinkWithBindings(t *testing.T) {
	c := compileAQL(t, multiLinkMutationSchema, `
with (
  new_members := (multi select User filter .department = 'Product');
)
update Application filter .id = $id<uuid> set {
  members := { "+": new_members }
};`)

	if !strings.Contains(c.SQL, `_with_new_members AS (`) {
		t.Errorf("expected with CTE _with_new_members:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `_ins_members AS (`) {
		t.Errorf("expected _ins_members CTE:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `SELECT id FROM "_with_new_members"`) {
		t.Errorf("expected insert from _with_new_members:\n%s", c.SQL)
	}
}

func TestInsertMultiLink(t *testing.T) {
	c := compileAQL(t, multiLinkMutationSchema, `
insert Application {
  name := $name<str>,
  members := { "+": (select User filter .email = $owner_email<str>) }
};`)

	if !strings.Contains(c.SQL, `_target AS (`) || !strings.Contains(c.SQL, `_ins_members AS (`) {
		t.Fatalf("expected CTE pipeline for insert with multi-link:\n%s", c.SQL)
	}
	if !strings.Contains(c.SQL, `INSERT INTO "application_members" ("application", "user")`) {
		t.Errorf("expected junction table insert on application_members:\n%s", c.SQL)
	}
}

func TestDeltaOnScalarFieldRejected(t *testing.T) {
	err := compileErr(t, multiLinkMutationSchema, `
update Application filter .id = $id<uuid> set {
  name := { "+": 'bad' }
};`)

	if err == nil || !strings.Contains(err.Error(), "non-multi") {
		t.Errorf("expected error on non-multi delta assignment, got %v", err)
	}
}

func TestDeltaOnSingleLinkRejected(t *testing.T) {
	err := compileErr(t, multiLinkMutationSchema, `
update Application filter .id = $id<uuid> set {
  owner := { "+": (select User filter .id = $uid) }
};`)

	if err == nil || (!strings.Contains(err.Error(), "non-multi") && !strings.Contains(err.Error(), "single link")) {
		t.Errorf("expected error on single link delta assignment, got %v", err)
	}
}
