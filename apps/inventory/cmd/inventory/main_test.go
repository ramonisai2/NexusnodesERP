package main

import (
	"context"
	"testing"
)

func TestPostMovementOptimisticAndIdempotent(t *testing.T) {
	store := NewMemoryStore()
	_ = store.SeedDemo()

	v := 1
	mov, err := store.PostMovement(context.Background(), MovementRequest{
		OrgID: "org_demo", BranchID: "br_norte", WarehouseID: "wh_norte", SKUID: "sku_bolt",
		MovementType: "ISSUE", Quantity: 1, ExpectedVersion: &v, IdempotencyKey: "idem-1", PostedBy: "usr_a",
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if mov.Status != "POSTED" {
		t.Fatalf("status=%s", mov.Status)
	}

	mov2, err := store.PostMovement(context.Background(), MovementRequest{
		OrgID: "org_demo", BranchID: "br_norte", WarehouseID: "wh_norte", SKUID: "sku_bolt",
		MovementType: "ISSUE", Quantity: 1, ExpectedVersion: &v, IdempotencyKey: "idem-1", PostedBy: "usr_a",
	})
	if err != nil {
		t.Fatalf("idem: %v", err)
	}
	if mov2.ID != mov.ID {
		t.Fatal("idempotency should return same movement")
	}

	_, err = store.PostMovement(context.Background(), MovementRequest{
		OrgID: "org_demo", BranchID: "br_norte", WarehouseID: "wh_norte", SKUID: "sku_bolt",
		MovementType: "ISSUE", Quantity: 1, ExpectedVersion: &v, IdempotencyKey: "idem-2", PostedBy: "usr_a",
	})
	if err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}
