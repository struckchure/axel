package asl

import "testing"

func TestConstraintPlainExclusiveHasNoFilter(t *testing.T) {
	ir := resolveSrc(t, `
type Account {
  required email: str;
  required tenant_id: str;
  constraint exclusive on (.email, .tenant_id);
}`)
	acct := ir.ObjectTypes["Account"]
	if acct == nil || len(acct.Constraints) != 1 {
		t.Fatalf("want 1 constraint, got %+v", acct)
	}
	tc := acct.Constraints[0]
	if tc.Expression != "exclusive" || len(tc.Columns) != 2 {
		t.Fatalf("constraint = %+v", tc)
	}
	if tc.FilterAQL != "" {
		t.Errorf("FilterAQL = %q, want empty", tc.FilterAQL)
	}
}

func TestConstraintPartialExclusiveCapturesFilter(t *testing.T) {
	ir := resolveSrc(t, `
enum QueueStatus { Pending, Done }
type Job {
  required name: str;
  required actor: str;
  required status: QueueStatus;
  constraint exclusive on (.name, .actor) filter .status = QueueStatus.Pending;
}`)
	job := ir.ObjectTypes["Job"]
	if job == nil || len(job.Constraints) != 1 {
		t.Fatalf("want 1 constraint, got %+v", job)
	}
	tc := job.Constraints[0]
	if tc.Expression != "exclusive" {
		t.Fatalf("constraint = %+v", tc)
	}
	if len(tc.Columns) != 2 || tc.Columns[0] != "name" || tc.Columns[1] != "actor" {
		t.Errorf("columns = %v", tc.Columns)
	}
	if tc.FilterAQL != `.status = QueueStatus.Pending` {
		t.Errorf("FilterAQL = %q", tc.FilterAQL)
	}
}
