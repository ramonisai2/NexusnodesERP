package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Subject struct {
	Sub         string         `json:"sub"`
	OrgID       string         `json:"org_id"`
	BranchIDs   []string       `json:"branch_ids"`
	Roles       []string       `json:"roles"`
	Permissions []string       `json:"permissions"`
	Attrs       map[string]any `json:"attrs"`
	AMR         []string       `json:"amr"`
}

func (s Subject) HasPermission(code string) bool {
	for _, p := range s.Permissions {
		if p == code {
			return true
		}
	}
	for _, r := range s.Roles {
		if r == "platform_admin" {
			return true
		}
	}
	return false
}

func (s Subject) HasBranch(branchID string) bool {
	for _, b := range s.BranchIDs {
		if b == branchID || b == "*" {
			return true
		}
	}
	return false
}

func (s Subject) MFALevel() int {
	return len(s.AMR)
}

// FromGatewayHeaders rebuilds the subject forwarded by the API gateway.
func FromGatewayHeaders(r *http.Request) Subject {
	attrs := map[string]any{}
	if raw := r.Header.Get("X-Attrs-JSON"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &attrs)
	}
	amr := splitCSV(r.Header.Get("X-Amr"))
	if len(amr) == 0 && strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true") {
		amr = []string{"pwd", "otp"}
	}
	sub := Subject{
		Sub:         r.Header.Get("X-User-Id"),
		OrgID:       r.Header.Get("X-Org-Id"),
		BranchIDs:   splitCSV(r.Header.Get("X-Branch-Ids")),
		Roles:       splitCSV(r.Header.Get("X-Roles")),
		Permissions: splitCSV(r.Header.Get("X-Permissions")),
		Attrs:       attrs,
		AMR:         amr,
	}
	if sub.Sub == "" && strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true") {
		sub.Sub = "usr_dev_analyst"
		sub.OrgID = "org_demo"
		sub.BranchIDs = []string{"br_norte", "br_sur"}
		sub.Roles = []string{"inventory_clerk", "payroll_analyst"}
		sub.Permissions = []string{
			"inventory.movement.create",
			"inventory.balance.read",
			"payroll.run.prepare",
			"payroll.run.read",
		}
		sub.Attrs = map[string]any{"max_payroll_amount": 1000000.0, "max_adjustment": 10000.0}
		sub.AMR = []string{"pwd", "otp"}
	}
	return sub
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	softFail   bool
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
		softFail: strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true"),
	}
}

func NewClientFromEnv() *Client {
	return NewClient(os.Getenv("OPA_URL"))
}

type Input struct {
	Subject  Subject        `json:"subject"`
	Action   string         `json:"action"`
	Resource map[string]any `json:"resource"`
	Context  map[string]any `json:"context"`
}

type opaRequest struct {
	Input Input `json:"input"`
}

type opaResponse struct {
	Result struct {
		Allow bool `json:"allow"`
	} `json:"result"`
}

func (c *Client) Allow(ctx context.Context, in Input) (bool, error) {
	if in.Context == nil {
		in.Context = map[string]any{}
	}
	if _, ok := in.Context["mfa_level"]; !ok {
		in.Context["mfa_level"] = in.Subject.MFALevel()
	}
	if c.baseURL == "" {
		return localAllow(in), nil
	}
	body, err := json.Marshal(opaRequest{Input: in})
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/data/nexus/authz", strings.NewReader(string(body)))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		if c.softFail {
			return localAllow(in), nil
		}
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		if c.softFail {
			return localAllow(in), nil
		}
		return false, fmt.Errorf("opa status %d", res.StatusCode)
	}
	var out opaResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Result.Allow, nil
}

func localAllow(in Input) bool {
	if in.Action == "inventory.catalog.read" {
		if !in.Subject.HasPermission("inventory.catalog.read") && !in.Subject.HasPermission("inventory.balance.read") {
			return false
		}
	} else if in.Action == "inventory.label.read" {
		if !in.Subject.HasPermission("inventory.label.read") && !in.Subject.HasPermission("inventory.balance.read") {
			return false
		}
	} else if in.Action == "reporting.read" {
		if !in.Subject.HasPermission("reporting.read") &&
			!in.Subject.HasPermission("inventory.balance.read") &&
			!in.Subject.HasPermission("payroll.run.read") {
			return false
		}
	} else if in.Action == "reporting.image.read" {
		if !in.Subject.HasPermission("reporting.image.read") &&
			!in.Subject.HasPermission("reporting.read") &&
			!in.Subject.HasPermission("inventory.balance.read") {
			return false
		}
	} else if in.Action == "search.query" {
		if !in.Subject.HasPermission("search.query") &&
			!in.Subject.HasPermission("inventory.balance.read") &&
			!in.Subject.HasPermission("inventory.catalog.read") &&
			!in.Subject.HasPermission("reporting.image.read") &&
			!in.Subject.HasPermission("reporting.read") {
			return false
		}
	} else if in.Action == "search.reindex" {
		ok := in.Subject.HasPermission("search.reindex")
		for _, r := range in.Subject.Roles {
			if r == "platform_admin" {
				ok = true
			}
		}
		if !ok {
			return false
		}
	} else if in.Action == "inventory.warehouse.read" {
		if !in.Subject.HasPermission("inventory.warehouse.read") &&
			!in.Subject.HasPermission("inventory.balance.read") {
			return false
		}
	} else if in.Action == "inventory.receipt.read" {
		if !in.Subject.HasPermission("inventory.receipt.read") &&
			!in.Subject.HasPermission("inventory.balance.read") {
			return false
		}
	} else if in.Action == "inventory.receipt.create" || in.Action == "inventory.receipt.post" {
		if !in.Subject.HasPermission("inventory.receipt.create") &&
			!in.Subject.HasPermission("inventory.receipt.post") &&
			!in.Subject.HasPermission("inventory.movement.create") {
			return false
		}
	} else if !in.Subject.HasPermission(in.Action) {
		return false
	}
	if branch, ok := in.Resource["branch_id"].(string); ok && branch != "" {
		if !in.Subject.HasBranch(branch) {
			return false
		}
	}
	if preparedBy, ok := in.Resource["prepared_by"].(string); ok && preparedBy != "" && in.Action == "payroll.run.approve" {
		if preparedBy == in.Subject.Sub {
			return false
		}
	}
	if total, ok := asFloat(in.Resource["total_amount"]); ok && in.Action == "payroll.run.approve" {
		maxAmt, _ := asFloat(in.Subject.Attrs["max_payroll_amount"])
		if maxAmt > 0 && total > maxAmt {
			return false
		}
	}
	if qty, ok := asFloat(in.Resource["quantity"]); ok && in.Action == "inventory.movement.create" {
		maxAdj, _ := asFloat(in.Subject.Attrs["max_adjustment"])
		if maxAdj > 0 {
			if qty < 0 {
				qty = -qty
			}
			if qty > maxAdj {
				return false
			}
		}
	}
	if in.Action == "inventory.movement.void" {
		if !warehouseManaged(in) {
			return false
		}
	}
	return in.Subject.MFALevel() >= 1
}

func warehouseManaged(in Input) bool {
	for _, r := range in.Subject.Roles {
		if r == "platform_admin" {
			return true
		}
	}
	wh, _ := in.Resource["warehouse_id"].(string)
	if wh == "" {
		return false
	}
	managed := asStringSlice(in.Subject.Attrs["managed_warehouses"])
	for _, m := range managed {
		if m == wh || m == "*" {
			return true
		}
	}
	return false
}

func asStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func WriteForbidden(w http.ResponseWriter, action string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"forbidden","action":"` + action + `"}`))
}
