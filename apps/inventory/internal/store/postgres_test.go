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

func TestPostgresStoreLabels(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := db.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	s := NewPostgres(pool)

	labels, err := s.ListLabels(context.Background(), domain.LabelFilter{
		OrgRef: "org_demo", BranchCode: "br_norte", SKUCode: "CAAL686101YL1",
	})
	if err != nil {
		t.Fatalf("labels: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %#v", labels)
	}
	l := labels[0]
	if l.StoreDisplayName != "LA MARINA" || l.PublicDescription == "" {
		t.Fatalf("store copy missing: %#v", l)
	}
	if l.Barcode != "7450130556398" || l.MaterialCode != "CAAL686101YL1" {
		t.Fatalf("ids mismatch: %#v", l)
	}
	if l.SizeCode != "M" || l.ColorCode != "YL1" {
		t.Fatalf("size/color mismatch: %#v", l)
	}
	if l.PriceMode != domain.PriceModeCommon || l.EffectivePrice == nil || *l.EffectivePrice != 899 {
		t.Fatalf("effective common price: mode=%s eff=%v", l.PriceMode, l.EffectivePrice)
	}

	sur, err := s.ListLabels(context.Background(), domain.LabelFilter{
		OrgRef: "org_demo", BranchCode: "br_sur", SKUCode: "7450130556398",
	})
	if err != nil || len(sur) != 1 {
		t.Fatalf("sur labels: %v %#v", err, sur)
	}
	if sur[0].PriceMode != domain.PriceModeSpecial || sur[0].EffectivePrice == nil || *sur[0].EffectivePrice != 699 {
		t.Fatalf("sur special price: %#v", sur[0])
	}
}
