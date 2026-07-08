package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrConflict          = errors.New("version conflict")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrNotFound          = errors.New("not found")
	ErrAlreadyVoided     = errors.New("already voided")
)

type StockBalance struct {
	ID           string   `json:"id"`
	WarehouseID  string   `json:"warehouse_id"` // public code
	BranchID     string   `json:"branch_id"`    // public code
	SKUID        string   `json:"sku_id"`       // public sku code
	SKU          string   `json:"sku"`
	ProductName  string   `json:"product_name,omitempty"`
	OnHand       float64  `json:"on_hand"`
	Reserved     float64  `json:"reserved"`
	Version      int      `json:"version"`
	Departments  []string `json:"departments,omitempty"`  // dept codes where article appears
	Categories   []string `json:"categories,omitempty"`   // category codes
	Placements   []string `json:"placements,omitempty"`   // "dept/category" labels
}

type BalanceFilter struct {
	OrgRef         string
	BranchCode     string
	DepartmentCode string
	CategoryCode   string
}

type DepartmentCategory struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type Department struct {
	Code       string                `json:"code"`
	Name       string                `json:"name"`
	BranchID   string                `json:"branch_id"`
	SortOrder  int                   `json:"sort_order"`
	Categories []DepartmentCategory  `json:"categories"`
}

type CatalogItem struct {
	ProductID   string             `json:"product_id"`
	SKUBase     string             `json:"sku_base"`
	Name        string             `json:"name"`
	SKUs        []string           `json:"skus"`
	Placements  []CatalogPlacement `json:"placements"`
}

type CatalogPlacement struct {
	DepartmentCode string `json:"department_code"`
	DepartmentName string `json:"department_name"`
	CategoryCode   string `json:"category_code,omitempty"`
	CategoryName   string `json:"category_name,omitempty"`
	IsPrimary      bool   `json:"is_primary"`
}

type CatalogFilter struct {
	OrgRef         string
	BranchCode     string
	DepartmentCode string
	CategoryCode   string
}

// PriceMode selects which store price the label printer should show.
const (
	PriceModeCommon  = "COMMON"
	PriceModeSpecial = "SPECIAL"
	PriceModeFinal   = "FINAL" // clearance until stock out
)

type LabelFilter struct {
	OrgRef         string
	BranchCode     string
	SKUCode        string
	DepartmentCode string
}

// StoreLabel is the payload a label printer needs per store + SKU.
type StoreLabel struct {
	ID                string         `json:"id"`
	BranchID          string         `json:"branch_id"`
	StoreDisplayName  string         `json:"store_display_name"`
	SKU               string         `json:"sku"`
	MaterialCode      string         `json:"material_code"`
	Barcode           string         `json:"barcode"`
	SizeCode          string         `json:"size_code,omitempty"`
	ColorCode         string         `json:"color_code,omitempty"`
	Brand             string         `json:"brand,omitempty"`
	PublicDescription string         `json:"public_description"`
	DepartmentLabel   string         `json:"department_label,omitempty"`
	ExtraDescriptions map[string]any `json:"extra_descriptions,omitempty"`
	Currency          string         `json:"currency"`
	CommonPrice       *float64       `json:"common_price,omitempty"`
	SpecialPrice      *float64       `json:"special_price,omitempty"`
	FinalPrice        *float64       `json:"final_price,omitempty"`
	PriceMode         string         `json:"price_mode"`
	EffectivePrice    *float64       `json:"effective_price,omitempty"`
	PriceLabel        string         `json:"price_label,omitempty"` // human: Común / Especial / Final
	OnHand            *float64       `json:"on_hand,omitempty"`
}

// EffectivePrice returns the price the printer should print for the active mode.
func (l StoreLabel) ResolveEffectivePrice() *float64 {
	switch l.PriceMode {
	case PriceModeSpecial:
		if l.SpecialPrice != nil {
			return l.SpecialPrice
		}
	case PriceModeFinal:
		if l.FinalPrice != nil {
			return l.FinalPrice
		}
	}
	return l.CommonPrice
}

type MovementRequest struct {
	OrgID           string  `json:"org_id"`
	BranchID        string  `json:"branch_id"`
	WarehouseID     string  `json:"warehouse_id"`
	SKUID           string  `json:"sku_id"`
	MovementType    string  `json:"movement_type"`
	Quantity        float64 `json:"quantity"`
	ExpectedVersion *int    `json:"expected_version"`
	IdempotencyKey  string  `json:"idempotency_key"`
	PostedBy        string  `json:"posted_by"` // idp_sub or user uuid
	OperatorLabel   string  `json:"operator_label,omitempty"`
	SessionID       string  `json:"session_id,omitempty"`
}

type Movement struct {
	ID             string     `json:"id"`
	OrgID          string     `json:"org_id"`
	BranchID       string     `json:"branch_id"`
	WarehouseID    string     `json:"warehouse_id"`
	SKUID          string     `json:"sku_id"`
	MovementType   string     `json:"movement_type"`
	Quantity       float64    `json:"quantity"`
	Status         string     `json:"status"`
	PostedBy       string     `json:"posted_by"`
	OperatorLabel  string     `json:"operator_label,omitempty"`
	SessionID      string     `json:"session_id,omitempty"`
	IdempotencyKey string     `json:"idempotency_key"`
	CreatedAt      time.Time  `json:"created_at"`
	ReversalOf     string     `json:"reversal_of,omitempty"`
	VoidReason     string     `json:"void_reason,omitempty"`
	VoidedBy       string     `json:"voided_by,omitempty"`
	VoidedAt       *time.Time `json:"voided_at,omitempty"`
}

type MovementFilter struct {
	OrgRef      string
	BranchCode  string
	WarehouseID string
	Limit       int
}

type VoidRequest struct {
	OrgID          string `json:"org_id"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
	VoidedBy       string `json:"voided_by"`
	OperatorLabel  string `json:"operator_label,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
}

type VoidResult struct {
	Original     Movement `json:"original"`
	Compensation Movement `json:"compensation"`
}

// WarehouseKind classifies stock locations for CEDI vs store vs small-shop arrival.
const (
	WarehouseKindStore   = "STORE"
	WarehouseKindCEDI    = "CEDI"
	WarehouseKindArrival = "ARRIVAL"
)

type Warehouse struct {
	ID       string `json:"id"`   // public code
	BranchID string `json:"branch_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // STORE | CEDI | ARRIVAL
}

type WarehouseFilter struct {
	OrgRef     string
	BranchCode string
	Kind       string
}

const (
	ReceiptStatusDraft  = "DRAFT"
	ReceiptStatusPosted = "POSTED"
	ReceiptStatusVoid   = "VOID"
)

type ReceiptLineInput struct {
	SKU              string   `json:"sku"`
	Quantity         float64  `json:"quantity"`
	UnitCost         *float64 `json:"unit_cost,omitempty"`
	LabelDescription string   `json:"label_description,omitempty"`
	LabelPrice       *float64 `json:"label_price,omitempty"`
}

type CreateReceiptRequest struct {
	OrgID          string             `json:"org_id"`
	BranchID       string             `json:"branch_id"`
	WarehouseID    string             `json:"warehouse_id"`
	SupplierName   string             `json:"supplier_name"`
	InvoiceNumber  string             `json:"invoice_number"`
	InvoiceDate    string             `json:"invoice_date,omitempty"` // YYYY-MM-DD
	Notes          string             `json:"notes,omitempty"`
	PrintLabels    *bool              `json:"print_labels,omitempty"`
	IdempotencyKey string             `json:"idempotency_key"`
	CreatedBy      string             `json:"created_by"`
	Lines          []ReceiptLineInput `json:"lines"`
}

type ReceiptLine struct {
	ID               string   `json:"id"`
	SKU              string   `json:"sku"`
	Quantity         float64  `json:"quantity"`
	UnitCost         *float64 `json:"unit_cost,omitempty"`
	LabelDescription string   `json:"label_description,omitempty"`
	LabelPrice       *float64 `json:"label_price,omitempty"`
	MovementID       string   `json:"movement_id,omitempty"`
	SortOrder        int      `json:"sort_order"`
}

type Receipt struct {
	ID             string        `json:"id"`
	OrgID          string        `json:"org_id"`
	BranchID       string        `json:"branch_id"`
	WarehouseID    string        `json:"warehouse_id"`
	WarehouseKind  string        `json:"warehouse_kind,omitempty"`
	SupplierName   string        `json:"supplier_name"`
	InvoiceNumber  string        `json:"invoice_number"`
	InvoiceDate    *string       `json:"invoice_date,omitempty"`
	Notes          string        `json:"notes,omitempty"`
	Status         string        `json:"status"`
	PrintLabels    bool          `json:"print_labels"`
	PostedAt       *time.Time    `json:"posted_at,omitempty"`
	PostedBy       string        `json:"posted_by,omitempty"`
	CreatedBy      string        `json:"created_by,omitempty"`
	IdempotencyKey string        `json:"idempotency_key"`
	CreatedAt      time.Time     `json:"created_at"`
	Lines          []ReceiptLine `json:"lines,omitempty"`
	Labels         []StoreLabel  `json:"labels,omitempty"`
}

type ReceiptFilter struct {
	OrgRef      string
	BranchCode  string
	WarehouseID string
	Status      string
	Limit       int
}

type Store interface {
	ListBalances(ctx context.Context, filter BalanceFilter) ([]StockBalance, error)
	PostMovement(ctx context.Context, req MovementRequest) (Movement, error)
	ListMovements(ctx context.Context, filter MovementFilter) ([]Movement, error)
	GetMovement(ctx context.Context, orgRef, movementID string) (Movement, error)
	VoidMovement(ctx context.Context, movementID string, req VoidRequest) (VoidResult, error)
	ListDepartments(ctx context.Context, orgRef, branchCode string) ([]Department, error)
	ListCatalog(ctx context.Context, filter CatalogFilter) ([]CatalogItem, error)
	ListLabels(ctx context.Context, filter LabelFilter) ([]StoreLabel, error)
	ListWarehouses(ctx context.Context, filter WarehouseFilter) ([]Warehouse, error)
	CreateReceipt(ctx context.Context, req CreateReceiptRequest) (Receipt, error)
	ListReceipts(ctx context.Context, filter ReceiptFilter) ([]Receipt, error)
	GetReceipt(ctx context.Context, orgRef, receiptID string) (Receipt, error)
	PostReceipt(ctx context.Context, orgRef, receiptID, postedBy string) (Receipt, error)
}
