package authz_test

import (
	"context"
	"testing"

	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
)

func TestLocalAllowSoDAndBranch(t *testing.T) {
	c := authz.NewClient("")
	subject := authz.Subject{
		Sub:         "usr_dev_dual",
		OrgID:       "org_demo",
		BranchIDs:   []string{"br_norte"},
		Roles:       []string{"payroll_approver"},
		Permissions: []string{"payroll.run.approve"},
		Attrs:       map[string]any{"max_payroll_amount": 1000000.0},
		AMR:         []string{"pwd", "otp"},
	}
	allow, err := c.Allow(context.Background(), authz.Input{
		Subject: subject,
		Action:  "payroll.run.approve",
		Resource: map[string]any{
			"branch_id":    "br_norte",
			"prepared_by":  "usr_dev_dual",
			"total_amount": 100.0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if allow {
		t.Fatal("SoD should deny")
	}

	allow, err = c.Allow(context.Background(), authz.Input{
		Subject: subject,
		Action:  "payroll.run.approve",
		Resource: map[string]any{
			"branch_id":    "br_norte",
			"prepared_by":  "usr_dev_analyst",
			"total_amount": 100.0,
		},
	})
	if err != nil || !allow {
		t.Fatalf("expected allow got %v %v", allow, err)
	}
}

func TestLocalAllowMovementVoidByManagedWarehouse(t *testing.T) {
	c := authz.NewClient("")
	manager := authz.Subject{
		Sub:         "usr_dev_wh_manager",
		OrgID:       "org_demo",
		BranchIDs:   []string{"br_norte"},
		Roles:       []string{"warehouse_manager"},
		Permissions: []string{"inventory.movement.void"},
		Attrs:       map[string]any{"managed_warehouses": []any{"wh_norte"}},
		AMR:         []string{"pwd", "otp"},
	}

	allow, err := c.Allow(context.Background(), authz.Input{
		Subject:  manager,
		Action:   "inventory.movement.void",
		Resource: map[string]any{"branch_id": "br_norte", "warehouse_id": "wh_norte"},
	})
	if err != nil || !allow {
		t.Fatalf("manager should void own warehouse: %v %v", allow, err)
	}

	allow, err = c.Allow(context.Background(), authz.Input{
		Subject:  manager,
		Action:   "inventory.movement.void",
		Resource: map[string]any{"branch_id": "br_sur", "warehouse_id": "wh_sur"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if allow {
		t.Fatal("manager must not void other area warehouse")
	}

	clerk := authz.Subject{
		Sub:         "usr_dev_analyst",
		OrgID:       "org_demo",
		BranchIDs:   []string{"br_norte"},
		Roles:       []string{"inventory_clerk"},
		Permissions: []string{"inventory.movement.create", "inventory.balance.read"},
		Attrs:       map[string]any{},
		AMR:         []string{"pwd", "otp"},
	}
	allow, err = c.Allow(context.Background(), authz.Input{
		Subject:  clerk,
		Action:   "inventory.movement.void",
		Resource: map[string]any{"branch_id": "br_norte", "warehouse_id": "wh_norte"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if allow {
		t.Fatal("clerk must not void")
	}
}
