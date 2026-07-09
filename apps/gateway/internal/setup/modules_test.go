package setup

import (
	"slices"
	"testing"
)

func TestNormalizeEnabledModulesForcesInventory(t *testing.T) {
	got := NormalizeEnabledModules([]string{"pos", "mail"})
	if !slices.Contains(got, "inventory") {
		t.Fatalf("expected inventory required, got %#v", got)
	}
	if !slices.Contains(got, "pos") || !slices.Contains(got, "mail") {
		t.Fatalf("expected pos+mail, got %#v", got)
	}
}

func TestPermissionsForModulesOmitsLocked(t *testing.T) {
	perms := PermissionsForModules([]string{"inventory", "pos"})
	has := map[string]bool{}
	for _, p := range perms {
		has[p] = true
	}
	if !has["inventory.balance.read"] || !has["pos.sale.create"] {
		t.Fatalf("missing core perms: %#v", perms)
	}
	if has["mail.read"] || has["payroll.run.read"] {
		t.Fatalf("locked modules leaked permissions: %#v", perms)
	}
}

func TestDefaultEnabledModules(t *testing.T) {
	defs := DefaultEnabledModules()
	if !slices.Contains(defs, "inventory") || !slices.Contains(defs, "pos") {
		t.Fatalf("unexpected defaults %#v", defs)
	}
	if slices.Contains(defs, "hr") {
		t.Fatalf("hr should be off by default %#v", defs)
	}
}
