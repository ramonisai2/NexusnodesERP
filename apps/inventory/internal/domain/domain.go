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
}

type VoidResult struct {
	Original     Movement `json:"original"`
	Compensation Movement `json:"compensation"`
}

type Store interface {
	ListBalances(ctx context.Context, filter BalanceFilter) ([]StockBalance, error)
	PostMovement(ctx context.Context, req MovementRequest) (Movement, error)
	ListMovements(ctx context.Context, filter MovementFilter) ([]Movement, error)
	GetMovement(ctx context.Context, orgRef, movementID string) (Movement, error)
	VoidMovement(ctx context.Context, movementID string, req VoidRequest) (VoidResult, error)
	ListDepartments(ctx context.Context, orgRef, branchCode string) ([]Department, error)
	ListCatalog(ctx context.Context, filter CatalogFilter) ([]CatalogItem, error)
}
