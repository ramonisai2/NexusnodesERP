package enrich_test

import (
	"context"
	"os"
	"testing"

	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/auth"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/enrich"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

func TestEnrichFromDB(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := db.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	e := enrich.New(pool)
	out, err := e.Enrich(context.Background(), auth.Claims{
		Sub:   "usr_dev_analyst",
		OrgID: "org_demo",
		AMR:   []string{"pwd", "otp"},
	})
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if len(out.Roles) == 0 {
		t.Fatal("expected roles from DB")
	}
	if len(out.Permissions) == 0 {
		t.Fatal("expected permissions from DB")
	}
	if len(out.BranchIDs) == 0 {
		t.Fatal("expected branches from DB")
	}
	found := false
	for _, p := range out.Permissions {
		if p == "inventory.movement.create" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing inventory.movement.create in %#v", out.Permissions)
	}
}
