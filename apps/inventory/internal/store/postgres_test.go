package store

import (
	"context"
	"os"
	"testing"

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

	balances, err := s.ListBalances(context.Background(), "br_norte")
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

	key := "test-idem-" + target.ID + "-" + string(rune(target.Version+'0'))
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
