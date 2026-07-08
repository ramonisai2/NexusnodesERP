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

func TestVoidMovementCompensatesAndIdempotent(t *testing.T) {
	s := store.NewMemory()
	s.SeedDemo()

	v := 1
	mov, err := s.PostMovement(context.Background(), domain.MovementRequest{
		OrgID: "org_demo", BranchID: "br_norte", WarehouseID: "wh_norte", SKUID: "BOLT-M8",
		MovementType: "ISSUE", Quantity: 5, ExpectedVersion: &v, IdempotencyKey: "void-src", PostedBy: "usr_clerk",
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}

	result, err := s.VoidMovement(context.Background(), mov.ID, domain.VoidRequest{
		OrgID: "org_demo", Reason: "Error de captura", IdempotencyKey: "void-1", VoidedBy: "usr_dev_wh_manager",
	})
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	if result.Original.Status != "VOID" {
		t.Fatalf("original status=%s", result.Original.Status)
	}
	if result.Compensation.Quantity != 5 {
		t.Fatalf("compensation qty=%v want 5", result.Compensation.Quantity)
	}
	if result.Compensation.ReversalOf != mov.ID {
		t.Fatal("compensation should link to original")
	}

	again, err := s.VoidMovement(context.Background(), mov.ID, domain.VoidRequest{
		OrgID: "org_demo", Reason: "Error de captura", IdempotencyKey: "void-1", VoidedBy: "usr_dev_wh_manager",
	})
	if err != nil {
		t.Fatalf("void idem: %v", err)
	}
	if again.Compensation.ID != result.Compensation.ID {
		t.Fatal("void idempotency should return same compensation")
	}

	_, err = s.VoidMovement(context.Background(), mov.ID, domain.VoidRequest{
		OrgID: "org_demo", Reason: "otra", IdempotencyKey: "void-2", VoidedBy: "usr_dev_wh_manager",
	})
	if err != domain.ErrAlreadyVoided {
		t.Fatalf("expected already voided, got %v", err)
	}

	list, err := s.ListMovements(context.Background(), domain.MovementFilter{BranchCode: "br_norte", Limit: 10})
	if err != nil || len(list) < 2 {
		t.Fatalf("list movements: %v len=%d", err, len(list))
	}
}

func TestDepartmentsAndMultiPlacementFilter(t *testing.T) {
	s := store.NewMemory()
	s.SeedDemo()

	deps, err := s.ListDepartments(context.Background(), "org_demo", "br_norte")
	if err != nil {
		t.Fatalf("departments: %v", err)
	}
	if len(deps) < 2 {
		t.Fatalf("expected electronica + jugueteria, got %#v", deps)
	}

	all, err := s.ListBalances(context.Background(), domain.BalanceFilter{BranchCode: "br_norte"})
	if err != nil {
		t.Fatalf("balances: %v", err)
	}
	if len(all) < 4 {
		t.Fatalf("expected retail + warehouse skus, got %d", len(all))
	}

	toys, err := s.ListBalances(context.Background(), domain.BalanceFilter{
		BranchCode: "br_norte", DepartmentCode: "jugueteria",
	})
	if err != nil {
		t.Fatalf("toys filter: %v", err)
	}
	if len(toys) != 1 || toys[0].SKUID != "FIG-COL-01" {
		t.Fatalf("collectible should appear in jugueteria, got %#v", toys)
	}

	games, err := s.ListBalances(context.Background(), domain.BalanceFilter{
		BranchCode: "br_norte", DepartmentCode: "electronica", CategoryCode: "videojuegos",
	})
	if err != nil {
		t.Fatalf("games filter: %v", err)
	}
	if len(games) != 1 || games[0].SKUID != "FIG-COL-01" {
		t.Fatalf("same collectible should appear in electronica/videojuegos, got %#v", games)
	}

	catalog, err := s.ListCatalog(context.Background(), domain.CatalogFilter{
		BranchCode: "br_norte", DepartmentCode: "jugueteria",
	})
	if err != nil || len(catalog) != 1 {
		t.Fatalf("catalog jugueteria: %v %#v", err, catalog)
	}
	if len(catalog[0].Placements) < 1 || catalog[0].Placements[0].DepartmentCode != "jugueteria" {
		t.Fatalf("expected jugueteria placement, got %#v", catalog[0].Placements)
	}
}
