package tests

import (
	"strings"
	"testing"
)

func TestLinkExclusiveEmitsUnique(t *testing.T) {
	up := genUp(t, `
type Application {
  required id: uuid { constraint pk; };
}
type Thing {
  required id: uuid { constraint pk; };
  required link application: Application { constraint exclusive; };
}
`)

	// The FK column carries UNIQUE inline...
	if !strings.Contains(up, `"application" UUID NOT NULL UNIQUE`) {
		t.Errorf("up SQL missing UNIQUE on link column:\n%s", up)
	}
	// ...and still emits the foreign key row.
	if !strings.Contains(up, `FOREIGN KEY ("application") REFERENCES "application"("id") ON DELETE CASCADE`) {
		t.Errorf("up SQL missing FK for link column:\n%s", up)
	}
}

func TestAddLinkWithExclusiveToExistingTable(t *testing.T) {
	base := `
type Application { required id: uuid { constraint pk; }; }
type Thing { required id: uuid { constraint pk; }; }
`
	withLink := `
type Application { required id: uuid { constraint pk; }; }
type Thing {
  required id: uuid { constraint pk; };
  required link application: Application { constraint exclusive; };
}
`
	up, _ := genMigration(t, base, withLink)

	if !strings.Contains(up, "ADD COLUMN application") || !strings.Contains(up, "UNIQUE") {
		t.Errorf("up SQL missing add-column with UNIQUE:\n%s", up)
	}
}
