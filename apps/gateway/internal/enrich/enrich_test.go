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

func TestEnrichManagedWarehouses(t *testing.T) {
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
		Sub:   "usr_dev_wh_manager",
		OrgID: "org_demo",
		AMR:   []string{"pwd", "otp"},
	})
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	foundVoid := false
	for _, p := range out.Permissions {
		if p == "inventory.movement.void" {
			foundVoid = true
		}
	}
	if !foundVoid {
		t.Fatalf("expected inventory.movement.void, got %#v", out.Permissions)
	}
	managed, ok := out.Attrs["managed_warehouses"].([]string)
	if !ok {
		// JSON round-trip may yield []any depending on path; accept both
		if raw, okAny := out.Attrs["managed_warehouses"].([]any); okAny {
			managed = nil
			for _, v := range raw {
				if s, ok := v.(string); ok {
					managed = append(managed, s)
				}
			}
		} else {
			t.Fatalf("managed_warehouses missing/type=%T val=%#v", out.Attrs["managed_warehouses"], out.Attrs["managed_warehouses"])
		}
	}
	hasNorte := false
	for _, w := range managed {
		if w == "wh_norte" {
			hasNorte = true
		}
		if w == "wh_sur" {
			t.Fatal("area manager must not manage wh_sur")
		}
	}
	if !hasNorte {
		t.Fatalf("expected wh_norte in %#v", managed)
	}

	regional, err := e.Enrich(context.Background(), auth.Claims{
		Sub:   "usr_dev_regional",
		OrgID: "org_demo",
		AMR:   []string{"pwd", "otp"},
	})
	if err != nil {
		t.Fatalf("enrich regional: %v", err)
	}
	rm, _ := regional.Attrs["managed_warehouses"].([]string)
	if rm == nil {
		if raw, ok := regional.Attrs["managed_warehouses"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					rm = append(rm, s)
				}
			}
		}
	}
	hasNorte = false
	for _, w := range rm {
		if w == "wh_norte" {
			hasNorte = true
		}
	}
	if !hasNorte {
		t.Fatalf("regional should manage wh_norte via hierarchy, got %#v", rm)
	}
}
