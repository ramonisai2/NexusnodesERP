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
