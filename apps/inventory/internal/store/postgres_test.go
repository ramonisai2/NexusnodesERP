package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

func TestPostgresPostMovement(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := db.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	s := NewPostgres(pool)

	balances, err := s.ListBalances(context.Background(), domain.BalanceFilter{
		OrgRef: "org_demo", BranchCode: "br_norte",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(balances) == 0 {
		t.Fatal("expected seeded balances")
	}
	var target *domain.StockBalance
	for i := range balances {
		if balances[i].SKU == "BOLT-M8" {
			target = &balances[i]
			break
		}
	}
	if target == nil {
		t.Fatal("BOLT-M8 not found")
	}

	key := fmt.Sprintf("test-idem-%d", time.Now().UnixNano())
	v := target.Version
	mov, err := s.PostMovement(context.Background(), domain.MovementRequest{
		OrgID: "org_demo", BranchID: "br_norte", WarehouseID: "wh_norte", SKUID: "BOLT-M8",
		MovementType: "ISSUE", Quantity: 1, ExpectedVersion: &v, IdempotencyKey: key, PostedBy: "usr_dev_analyst",
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if mov.Status != "POSTED" {
		t.Fatalf("status=%s", mov.Status)
	}
	mov2, err := s.PostMovement(context.Background(), domain.MovementRequest{
		OrgID: "org_demo", BranchID: "br_norte", WarehouseID: "wh_norte", SKUID: "BOLT-M8",
		MovementType: "ISSUE", Quantity: 1, ExpectedVersion: &v, IdempotencyKey: key, PostedBy: "usr_dev_analyst",
	})
	if err != nil {
		t.Fatalf("idem: %v", err)
	}
	if mov2.ID != mov.ID {
		t.Fatal("idem mismatch")
	}
}

func TestPostgresMultiDepartmentPlacement(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := db.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	s := NewPostgres(pool)

	deps, err := s.ListDepartments(context.Background(), "org_demo", "br_norte")
	if err != nil {
		t.Fatalf("departments: %v", err)
	}
	if len(deps) < 2 {
		t.Fatalf("expected departments, got %#v", deps)
	}

	toys, err := s.ListBalances(context.Background(), domain.BalanceFilter{
		OrgRef: "org_demo", BranchCode: "br_norte", DepartmentCode: "jugueteria",
	})
	if err != nil {
		t.Fatalf("jugueteria: %v", err)
	}
	found := false
	for _, b := range toys {
		if b.SKUID == "FIG-COL-01" {
			found = true
			if len(b.Placements) < 2 {
				t.Fatalf("collectible should list both placements, got %#v", b.Placements)
			}
		}
	}
	if !found {
		t.Fatal("FIG-COL-01 should appear under jugueteria")
	}

	elec, err := s.ListBalances(context.Background(), domain.BalanceFilter{
		OrgRef: "org_demo", BranchCode: "br_norte", DepartmentCode: "electronica", CategoryCode: "videojuegos",
	})
	if err != nil {
		t.Fatalf("videojuegos: %v", err)
	}
	found = false
	for _, b := range elec {
		if b.SKUID == "FIG-COL-01" {
			found = true
		}
	}
	if !found {
		t.Fatal("FIG-COL-01 should also appear under electronica/videojuegos")
	}

	catalog, err := s.ListCatalog(context.Background(), domain.CatalogFilter{
		OrgRef: "org_demo", BranchCode: "br_norte", DepartmentCode: "jugueteria",
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(catalog) == 0 {
		t.Fatal("expected catalog items in jugueteria")
	}
}
