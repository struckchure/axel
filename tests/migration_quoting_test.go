package tests

import (
	"strings"
	"testing"
)

func TestMigrationColumnQuoting(t *testing.T) {
	before := `
type Transaction {
  required id: uuid { constraint pk; };
  ledger: str;
  user: str;
}
`
	after := `
type Transaction {
  required id: uuid { constraint pk; };
  required ledger: str;
  narration: str;
  required reference: str {
    default := "ref_001";
    constraint exclusive;
  };
}
`

	up, down := genMigration(t, before, after)

	// In up:
	// 1. ledger modified to NOT NULL -> ALTER TABLE "transaction" ALTER COLUMN "ledger" SET NOT NULL;
	if !strings.Contains(up, `ALTER TABLE "transaction" ALTER COLUMN "ledger" SET NOT NULL;`) {
		t.Errorf("expected quoted ledger in SET NOT NULL, got:\n%s", up)
	}
	// 2. narration added -> ALTER TABLE "transaction" ADD COLUMN "narration" TEXT;
	if !strings.Contains(up, `ALTER TABLE "transaction" ADD COLUMN "narration" TEXT;`) {
		t.Errorf("expected quoted narration in ADD COLUMN, got:\n%s", up)
	}
	// 3. reference added -> ALTER TABLE "transaction" ADD COLUMN "reference" TEXT NOT NULL DEFAULT 'ref_001' CONSTRAINT "uq_transaction_reference" UNIQUE;
	if !strings.Contains(up, `ALTER TABLE "transaction" ADD COLUMN "reference"`) {
		t.Errorf("expected quoted reference in ADD COLUMN, got:\n%s", up)
	}
	// 4. user dropped -> ALTER TABLE "transaction" DROP COLUMN IF EXISTS "user";
	if !strings.Contains(up, `ALTER TABLE "transaction" DROP COLUMN IF EXISTS "user";`) {
		t.Errorf("expected quoted user in DROP COLUMN, got:\n%s", up)
	}

	// In down:
	// user added back -> ALTER TABLE "transaction" ADD COLUMN "user" TEXT;
	if !strings.Contains(down, `ALTER TABLE "transaction" ADD COLUMN "user" TEXT;`) {
		t.Errorf("expected quoted user in down ADD COLUMN, got:\n%s", down)
	}
	// narration dropped -> ALTER TABLE "transaction" DROP COLUMN IF EXISTS "narration";
	if !strings.Contains(down, `ALTER TABLE "transaction" DROP COLUMN IF EXISTS "narration";`) {
		t.Errorf("expected quoted narration in down DROP COLUMN, got:\n%s", down)
	}
	// reference dropped -> ALTER TABLE "transaction" DROP COLUMN IF EXISTS "reference";
	if !strings.Contains(down, `ALTER TABLE "transaction" DROP COLUMN IF EXISTS "reference";`) {
		t.Errorf("expected quoted reference in down DROP COLUMN, got:\n%s", down)
	}
	// ledger NOT NULL dropped -> ALTER TABLE "transaction" ALTER COLUMN "ledger" DROP NOT NULL;
	if !strings.Contains(down, `ALTER TABLE "transaction" ALTER COLUMN "ledger" DROP NOT NULL;`) {
		t.Errorf("expected quoted ledger in down DROP NOT NULL, got:\n%s", down)
	}
}
