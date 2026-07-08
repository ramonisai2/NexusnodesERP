package main

import (
	"context"
	"testing"

	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/store"
)

func TestPostMovementOptimisticAndIdempotent(t *testing.T) {
	s := store.NewMemory()
	s.SeedDemo()

	v := 1
	mov, err := s.PostMovement(context.Background(), domain.MovementRequest{
		OrgID: "org_demo", BranchID: "br_norte", WarehouseID: "wh_norte", SKUID: "BOLT-M8",
		MovementType: "ISSUE", Quantity: 1, ExpectedVersion: &v, IdempotencyKey: "idem-1", PostedBy: "usr_a",
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if mov.Status != "POSTED" {
		t.Fatalf("status=%s", mov.Status)
	}

	mov2, err := s.PostMovement(context.Background(), domain.MovementRequest{
		OrgID: "org_demo", BranchID: "br_norte", WarehouseID: "wh_norte", SKUID: "BOLT-M8",
		MovementType: "ISSUE", Quantity: 1, ExpectedVersion: &v, IdempotencyKey: "idem-1", PostedBy: "usr_a",
	})
	if err != nil {
		t.Fatalf("idem: %v", err)
	}
	if mov2.ID != mov.ID {
		t.Fatal("idempotency should return same movement")
	}

	_, err = s.PostMovement(context.Background(), domain.MovementRequest{
		OrgID: "org_demo", BranchID: "br_norte", WarehouseID: "wh_norte", SKUID: "BOLT-M8",
		MovementType: "ISSUE", Quantity: 1, ExpectedVersion: &v, IdempotencyKey: "idem-2", PostedBy: "usr_a",
	})
	if err != domain.ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}
