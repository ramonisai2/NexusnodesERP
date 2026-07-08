package store

import (
	"context"
	"os"
	"testing"

	"github.com/ramonisai2/NexusnodesERP/apps/payroll/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

func TestPostgresPayrollSoD(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := db.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	s := NewPostgres(pool)

	run, err := s.CreateAndCalculate(context.Background(), domain.CreateRunRequest{
		OrgID:       "org_demo",
		PeriodLabel: "2026-07-H1",
		BranchID:    "br_norte",
		PreparedBy:  "usr_dev_analyst",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = s.Approve(context.Background(), run.ID, "usr_dev_analyst")
	if err != domain.ErrSoDViolation {
		t.Fatalf("expected SoD got %v", err)
	}
	approved, err := s.Approve(context.Background(), run.ID, "usr_dev_approver")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != "APPROVED" {
		t.Fatalf("status=%s", approved.Status)
	}
}
