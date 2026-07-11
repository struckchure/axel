package tests

import (
	"strings"
	"testing"
)

func TestCompositeUniqueConstraint(t *testing.T) {
	up := genUp(t, `
type Account {
  required id: uuid { constraint pk; };
  required email: str;
  required tenant_id: uuid;
  constraint exclusive on (.email, .tenant_id);
}
`)
	want := `CONSTRAINT "uq_account_email_tenant_id" UNIQUE ("email", "tenant_id")`
	if !strings.Contains(up, want) {
		t.Errorf("up SQL missing composite unique:\n%s", up)
	}
}

func TestCompositePrimaryKeyConstraint(t *testing.T) {
	up := genUp(t, `
type Thing {
  required a: uuid;
  required b: uuid;
  constraint pk on (.a, .b);
}
`)
	want := `CONSTRAINT "pk_thing" PRIMARY KEY ("a", "b")`
	if !strings.Contains(up, want) {
		t.Errorf("up SQL missing composite primary key:\n%s", up)
	}
}

func TestTypeLevelLengthConstraint(t *testing.T) {
	up := genUp(t, `
type Product {
  required id: uuid { constraint pk; };
  required code: str;
  constraint min_length(3) on (.code);
}
`)
	want := `CONSTRAINT "chk_product_code_min_length" CHECK (char_length("code") >= 3)`
	if !strings.Contains(up, want) {
		t.Errorf("up SQL missing type-level length CHECK:\n%s", up)
	}
}

func TestAddConstraintToExistingTable(t *testing.T) {
	base := `type Account {
  required id: uuid { constraint pk; };
  required email: str;
  required tenant_id: uuid;
}`
	withC := `type Account {
  required id: uuid { constraint pk; };
  required email: str;
  required tenant_id: uuid;
  constraint exclusive on (.email, .tenant_id);
}`

	up, down := genMigration(t, base, withC)

	wantUp := `ALTER TABLE "account" ADD CONSTRAINT "uq_account_email_tenant_id" UNIQUE ("email", "tenant_id");`
	if !strings.Contains(up, wantUp) {
		t.Errorf("up SQL missing ADD CONSTRAINT:\n%s", up)
	}
	wantDown := `ALTER TABLE "account" DROP CONSTRAINT IF EXISTS "uq_account_email_tenant_id";`
	if !strings.Contains(down, wantDown) {
		t.Errorf("down SQL missing DROP CONSTRAINT:\n%s", down)
	}
}

func TestFieldLengthChecksInCreateTable(t *testing.T) {
	up := genUp(t, `
type User {
  required id: uuid { constraint pk; };
  required email: str {
    constraint exclusive;
    constraint min_length(6);
    constraint max_length(100);
  };
}
`)
	for _, want := range []string{
		`CONSTRAINT "uq_user_email" UNIQUE`,
		`CONSTRAINT "chk_user_email_min_length" CHECK (char_length("email") >= 6)`,
		`CONSTRAINT "chk_user_email_max_length" CHECK (char_length("email") <= 100)`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("up SQL missing %q:\n%s", want, up)
		}
	}
}

func TestLengthChecksSkippedForNonString(t *testing.T) {
	up := genUp(t, `
type User {
  required id: uuid { constraint pk; };
  required age: int32 { constraint min_length(6); };
}
`)
	if strings.Contains(up, `char_length("age")`) {
		t.Errorf("length CHECK should not apply to int32 column:\n%s", up)
	}
}

func TestCreateTableNamesForeignKey(t *testing.T) {
	up := genUp(t, `
type User { required id: uuid { constraint pk; }; }
type Post {
  required id: uuid { constraint pk; };
  required link author: User;
}
`)
	want := `CONSTRAINT "fk_post_author" FOREIGN KEY ("author") REFERENCES "user"("id") ON DELETE CASCADE`
	if !strings.Contains(up, want) {
		t.Errorf("up SQL missing named FK:\n%s", up)
	}
}

func TestJunctionTableNamesConstraints(t *testing.T) {
	up := genUp(t, `
type User { required id: uuid { constraint pk; }; }
type Post {
  required id: uuid { constraint pk; };
  multi link likes: User;
}
`)
	for _, want := range []string{
		`CONSTRAINT "pk_post_likes" PRIMARY KEY ("post", "user")`,
		`CONSTRAINT "fk_post_likes_post" FOREIGN KEY ("post")`,
		`CONSTRAINT "fk_post_likes_user" FOREIGN KEY ("user")`,
	} {
		if !strings.Contains(up, want) {
			t.Errorf("junction table missing %q:\n%s", want, up)
		}
	}
}

// The whole point of named constraints: the name a constraint is created with must
// be byte-identical to the name a later migration drops, or the DROP silently
// no-ops and leaves a stale constraint behind.
func TestCreateAndAlterConstraintNamesAgree(t *testing.T) {
	// Length CHECK: created with min_length, then removed.
	create := genUp(t, `type User { required id: uuid { constraint pk; }; required email: str { constraint min_length(6); }; }`)
	if !strings.Contains(create, `CONSTRAINT "chk_user_email_min_length"`) {
		t.Fatalf("CREATE missing named length check:\n%s", create)
	}
	dropUp, _ := genMigration(t,
		`type User { required id: uuid { constraint pk; }; required email: str { constraint min_length(6); }; }`,
		`type User { required id: uuid { constraint pk; }; required email: str; }`)
	if !strings.Contains(dropUp, `DROP CONSTRAINT IF EXISTS "chk_user_email_min_length"`) {
		t.Fatalf("ALTER drops a different name than CREATE made:\n%s", dropUp)
	}

	// Enum CHECK: created, then the backing enum removed.
	createEnum := genUp(t, `enum Role { Admin, Member } type User { required id: uuid { constraint pk; }; required role: Role; }`)
	if !strings.Contains(createEnum, `CONSTRAINT "chk_user_role_enum"`) {
		t.Fatalf("CREATE missing named enum check:\n%s", createEnum)
	}
	dropEnumUp, _ := genMigration(t,
		`enum Role { Admin, Member } type User { required id: uuid { constraint pk; }; required role: Role; }`,
		`type User { required id: uuid { constraint pk; }; required role: str; }`)
	if !strings.Contains(dropEnumUp, `DROP CONSTRAINT IF EXISTS "chk_user_role_enum"`) {
		t.Fatalf("ALTER drops a different enum name than CREATE made:\n%s", dropEnumUp)
	}
}

func TestModifyColumnAddsLengthConstraint(t *testing.T) {
	base := `type User { required id: uuid { constraint pk; }; required email: str; }`
	withLen := `type User { required id: uuid { constraint pk; }; required email: str { constraint min_length(6); }; }`

	up, down := genMigration(t, base, withLen)

	wantUp := `ALTER TABLE "user" ADD CONSTRAINT "chk_user_email_min_length" CHECK (char_length("email") >= 6);`
	if !strings.Contains(up, wantUp) {
		t.Errorf("up SQL missing add length constraint:\n%s", up)
	}
	wantDown := `ALTER TABLE "user" DROP CONSTRAINT IF EXISTS "chk_user_email_min_length";`
	if !strings.Contains(down, wantDown) {
		t.Errorf("down SQL missing drop length constraint:\n%s", down)
	}
}
