package setup

import "testing"

func TestNormalizeNetworkMode(t *testing.T) {
	if NormalizeNetworkMode("") != NetworkIntranet {
		t.Fatalf("empty should default intranet")
	}
	if NormalizeNetworkMode("INTERNET") != NetworkInternet {
		t.Fatalf("internet")
	}
	if NormalizeNetworkMode("public") != NetworkInternet {
		t.Fatalf("public alias")
	}
}

func TestApplyNetworkModeToModules(t *testing.T) {
	mods := []string{"inventory", "pos", "storefront", "customers"}
	got := ApplyNetworkModeToModules(NetworkIntranet, mods)
	for _, c := range got {
		if c == "storefront" {
			t.Fatalf("storefront must be forced off on intranet: %v", got)
		}
	}
	got2 := ApplyNetworkModeToModules(NetworkInternet, mods)
	found := false
	for _, c := range got2 {
		if c == "storefront" {
			found = true
		}
	}
	if !found {
		t.Fatalf("storefront should remain on internet: %v", got2)
	}
}

func TestAllowsPublicEgress(t *testing.T) {
	if AllowsPublicEgress(NetworkIntranet) {
		t.Fatal("intranet must not allow public egress")
	}
	if !AllowsPublicEgress(NetworkInternet) {
		t.Fatal("internet must allow public egress")
	}
}
