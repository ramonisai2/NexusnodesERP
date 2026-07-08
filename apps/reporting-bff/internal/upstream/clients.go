package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
)

type Client struct {
	inventoryBase string
	payrollBase   string
	http          *http.Client
}

func New(inventoryURL, payrollURL string) *Client {
	return &Client{
		inventoryBase: strings.TrimRight(inventoryURL, "/"),
		payrollBase:   strings.TrimRight(payrollURL, "/"),
		http:          otelx.HTTPClient(8 * time.Second),
	}
}

type Balance struct {
	ID          string   `json:"id"`
	WarehouseID string   `json:"warehouse_id"`
	BranchID    string   `json:"branch_id"`
	SKUID       string   `json:"sku_id"`
	SKU         string   `json:"sku"`
	ProductName string   `json:"product_name"`
	OnHand      float64  `json:"on_hand"`
	Reserved    float64  `json:"reserved"`
	Version     int      `json:"version"`
	Departments []string `json:"departments"`
	Categories  []string `json:"categories"`
	Placements  []string `json:"placements"`
}

type Department struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	BranchID   string `json:"branch_id"`
	SortOrder  int    `json:"sort_order"`
	Categories []struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"categories"`
}

type Label struct {
	ID                string   `json:"id"`
	BranchID          string   `json:"branch_id"`
	StoreDisplayName  string   `json:"store_display_name"`
	SKU               string   `json:"sku"`
	MaterialCode      string   `json:"material_code"`
	Barcode           string   `json:"barcode"`
	SizeCode          string   `json:"size_code"`
	ColorCode         string   `json:"color_code"`
	Brand             string   `json:"brand"`
	PublicDescription string   `json:"public_description"`
	DepartmentLabel   string   `json:"department_label"`
	Currency          string   `json:"currency"`
	CommonPrice       *float64 `json:"common_price"`
	SpecialPrice      *float64 `json:"special_price"`
	FinalPrice        *float64 `json:"final_price"`
	PriceMode         string   `json:"price_mode"`
	EffectivePrice    *float64 `json:"effective_price"`
	PriceLabel        string   `json:"price_label"`
	OnHand            *float64 `json:"on_hand"`
}

type PayrollRun struct {
	ID          string  `json:"id"`
	PeriodLabel string  `json:"period_label"`
	BranchID    string  `json:"branch_id"`
	Status      string  `json:"status"`
	PreparedBy  string  `json:"prepared_by"`
	ApprovedBy  string  `json:"approved_by"`
	TotalAmount float64 `json:"total_amount"`
	Version     int     `json:"version"`
}

func (c *Client) ListBalances(ctx context.Context, subject authz.Subject, branchID, department, category string) ([]Balance, error) {
	q := url.Values{}
	if branchID != "" {
		q.Set("branch_id", branchID)
	}
	if department != "" {
		q.Set("department", department)
	}
	if category != "" {
		q.Set("category", category)
	}
	path := "/balances"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out []Balance
	if err := c.getJSON(ctx, c.inventoryBase+path, subject, branchID, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListDepartments(ctx context.Context, subject authz.Subject, branchID string) ([]Department, error) {
	path := "/departments"
	if branchID != "" {
		path += "?branch_id=" + url.QueryEscape(branchID)
	}
	var out []Department
	if err := c.getJSON(ctx, c.inventoryBase+path, subject, branchID, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListLabels(ctx context.Context, subject authz.Subject, branchID, department string) ([]Label, error) {
	q := url.Values{}
	if branchID != "" {
		q.Set("branch_id", branchID)
	}
	if department != "" {
		q.Set("department", department)
	}
	path := "/labels"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out []Label
	if err := c.getJSON(ctx, c.inventoryBase+path, subject, branchID, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListPayrollRuns(ctx context.Context, subject authz.Subject, branchID string) ([]PayrollRun, error) {
	var out []PayrollRun
	if err := c.getJSON(ctx, c.payrollBase+"/runs", subject, branchID, &out); err != nil {
		return nil, err
	}
	if branchID == "" {
		return out, nil
	}
	filtered := make([]PayrollRun, 0, len(out))
	for _, r := range out {
		if r.BranchID == branchID {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func (c *Client) getJSON(ctx context.Context, rawURL string, subject authz.Subject, branchID string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-User-Id", subject.Sub)
	req.Header.Set("X-Org-Id", subject.OrgID)
	req.Header.Set("X-Branch-Ids", strings.Join(subject.BranchIDs, ","))
	req.Header.Set("X-Permissions", strings.Join(subject.Permissions, ","))
	req.Header.Set("X-Roles", strings.Join(subject.Roles, ","))
	req.Header.Set("X-Amr", strings.Join(subject.AMR, ","))
	if branchID != "" {
		req.Header.Set("X-Branch-Id", branchID)
	}
	if subject.Attrs != nil {
		if raw, err := json.Marshal(subject.Attrs); err == nil {
			req.Header.Set("X-Attrs-JSON", string(raw))
		}
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("upstream %s status %d: %s", rawURL, res.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 || string(body) == "null" {
		return nil
	}
	return json.Unmarshal(body, dest)
}
