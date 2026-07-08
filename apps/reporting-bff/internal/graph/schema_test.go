package graph_test

import (
	"context"
	"testing"

	"github.com/graphql-go/graphql"
	"github.com/ramonisai2/NexusnodesERP/apps/reporting-bff/internal/graph"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
)

func TestSchemaBuildsAndRejectsWithoutSubject(t *testing.T) {
	svc := &graph.Services{
		OPA: authz.NewClient(""),
	}
	schema, err := graph.NewSchema(svc)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ inventorySummary { skuCount } }`,
		Context:       context.Background(),
	})
	if len(result.Errors) == 0 {
		t.Fatal("expected missing subject error")
	}
}

func TestReportingAllowWithBalancePermission(t *testing.T) {
	c := authz.NewClient("")
	subject := authz.Subject{
		Sub:         "usr_dev_analyst",
		OrgID:       "org_demo",
		BranchIDs:   []string{"br_norte"},
		Permissions: []string{"inventory.balance.read"},
		AMR:         []string{"pwd", "otp"},
	}
	allow, err := c.Allow(context.Background(), authz.Input{
		Subject:  subject,
		Action:   "reporting.read",
		Resource: map[string]any{"branch_id": "br_norte"},
	})
	if err != nil || !allow {
		t.Fatalf("expected reporting allow via balance.read, got %v %v", allow, err)
	}
}
