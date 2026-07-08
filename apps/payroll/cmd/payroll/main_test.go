package main

import "testing"

func TestPayrollSoD(t *testing.T) {
	store := NewMemoryStore()
	_ = store.SeedDemo()

	run, err := store.CreateAndCalculate(CreateRunRequest{
		PeriodLabel: "2026-07-H1",
		BranchID:    "br_norte",
		PreparedBy:  "usr_dev_analyst",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if run.Status != "IN_REVIEW" {
		t.Fatalf("status=%s", run.Status)
	}

	_, err = store.Approve(run.ID, "usr_dev_analyst")
	if err != ErrSoDViolation {
		t.Fatalf("expected SoD, got %v", err)
	}

	approved, err := store.Approve(run.ID, "usr_dev_approver")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != "APPROVED" {
		t.Fatalf("status=%s", approved.Status)
	}
}
