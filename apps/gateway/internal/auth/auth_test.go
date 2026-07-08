package auth

import (
	"context"
	"testing"
	"time"
)

func TestHasPermissionAndBranch(t *testing.T) {
	c := DevClaims()
	if !c.HasPermission("inventory.movement.create") {
		t.Fatal("expected permission")
	}
	if c.HasPermission("does.not.exist") {
		t.Fatal("unexpected permission")
	}
	if !c.HasBranch("br_norte") {
		t.Fatal("expected branch")
	}
	if c.HasBranch("br_unknown") {
		t.Fatal("unexpected branch")
	}
}

func TestIssueAndParseDevToken(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "true")
	v := NewValidatorFromEnv()
	tok, err := v.IssueDevToken(DevClaims(), time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := v.Parse(context.Background(), tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Sub != "usr_dev_analyst" {
		t.Fatalf("sub=%s", claims.Sub)
	}
	if claims.OrgID != "org_demo" {
		t.Fatalf("org=%s", claims.OrgID)
	}
}
