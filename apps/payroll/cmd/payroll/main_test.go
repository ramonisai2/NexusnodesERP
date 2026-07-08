package main

import (
	"context"
	"testing"

	"github.com/ramonisai2/NexusnodesERP/apps/payroll/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/apps/payroll/internal/store"
)

func TestPayrollSoD(t *testing.T) {
	s := store.NewMemory()
	s.SeedDemo()

	run, err := s.CreateAndCalculate(context.Background(), domain.CreateRunRequest{
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

	_, err = s.Approve(context.Background(), "org_demo", run.ID, "usr_dev_analyst")
	if err != domain.ErrSoDViolation {
		t.Fatalf("expected SoD, got %v", err)
	}

	approved, err := s.Approve(context.Background(), "org_demo", run.ID, "usr_dev_approver")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != "APPROVED" {
		t.Fatalf("status=%s", approved.Status)
	}
}
