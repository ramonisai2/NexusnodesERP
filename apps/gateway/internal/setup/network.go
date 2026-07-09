package setup

import "strings"

const (
	NetworkIntranet = "intranet"
	NetworkInternet = "internet"
)

// NetworkModeDef describes an install-time network posture.
type NetworkModeDef struct {
	Code        string   `json:"code"`
	Default     bool     `json:"default"`
	LabelKey    string   `json:"label_key"`
	HintKey     string   `json:"hint_key"`
	DetailKey   string   `json:"detail_key"`
	ModulesOff  []string `json:"modules_forced_off,omitempty"`
	PublicAPIs  []string `json:"public_apis,omitempty"`
}

// NetworkModeCatalog returns the two clearly differentiated postures.
func NetworkModeCatalog() []NetworkModeDef {
	return []NetworkModeDef{
		{
			Code:      NetworkIntranet,
			Default:   true,
			LabelKey:  "netModeIntranet",
			HintKey:   "netModeIntranetHint",
			DetailKey: "netModeIntranetDetail",
			ModulesOff: InternetFacingModules(),
			PublicAPIs: []string{},
		},
		{
			Code:      NetworkInternet,
			Default:   false,
			LabelKey:  "netModeInternet",
			HintKey:   "netModeInternetHint",
			DetailKey: "netModeInternetDetail",
			PublicAPIs: []string{
				"GET /storefront/public/{slug}",
				"POST /customers/register",
				"POST /customers/login",
			},
		},
	}
}

// InternetFacingModules are install modules that imply public internet exposure.
// Staff-only CRM (customers) stays available on intranet; only the public
// storefront / self-service portal requires internet mode.
func InternetFacingModules() []string {
	return []string{"storefront"}
}

// NormalizeNetworkMode returns intranet|internet (default intranet).
func NormalizeNetworkMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case NetworkInternet, "public", "hybrid", "egress":
		return NetworkInternet
	default:
		return NetworkIntranet
	}
}

// AllowsPublicEgress reports whether public internet surfaces may be served.
func AllowsPublicEgress(mode string) bool {
	return NormalizeNetworkMode(mode) == NetworkInternet
}

// ApplyNetworkModeToModules forces internet-facing modules off on intranet.
func ApplyNetworkModeToModules(mode string, enabled []string) []string {
	enabled = NormalizeEnabledModules(enabled)
	if AllowsPublicEgress(mode) {
		return enabled
	}
	block := map[string]bool{}
	for _, c := range InternetFacingModules() {
		block[c] = true
	}
	out := make([]string, 0, len(enabled))
	for _, c := range enabled {
		if block[c] {
			continue
		}
		out = append(out, c)
	}
	return out
}
