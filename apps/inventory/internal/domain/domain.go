package domain

import (
	"context"
	"errors"
	"strings"
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

// Container types for inter-store papelería (identification slips stuck on packages).
const (
	ContainerEnvelope     = "ENVELOPE"      // sobre
	ContainerBox          = "BOX"           // caja
	ContainerPlasticBox   = "PLASTIC_BOX"   // caja plástica
	ContainerBundle       = "BUNDLE"        // bulto
	ContainerOriginalPack = "ORIGINAL_PACK" // empaque original
)

const (
	SlipStatusDraft     = "DRAFT"
	SlipStatusPrinted   = "PRINTED"
	SlipStatusInTransit = "IN_TRANSIT"
	SlipStatusReceived  = "RECEIVED"
	SlipStatusCancelled = "CANCELLED"
)

func ValidContainerType(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case ContainerEnvelope, ContainerBox, ContainerPlasticBox, ContainerBundle, ContainerOriginalPack:
		return true
	default:
		return false
	}
}

func ContainerTypeLabelES(code string) string {
	switch strings.ToUpper(code) {
	case ContainerEnvelope:
		return "Sobre"
	case ContainerBox:
		return "Caja"
	case ContainerPlasticBox:
		return "Caja plástica"
	case ContainerBundle:
		return "Bulto"
	case ContainerOriginalPack:
		return "Empaque original"
	default:
		return code
	}
}

type ShippingSlipLineInput struct {
	SKU         string  `json:"sku,omitempty"`
	Description string  `json:"description,omitempty"`
	Quantity    float64 `json:"quantity"`
}

type CreateShippingSlipRequest struct {
	OrgID           string                  `json:"org_id"`
	FromBranchID    string                  `json:"from_branch_id"`
	ToBranchID      string                  `json:"to_branch_id"`
	FromWarehouseID string                  `json:"from_warehouse_id,omitempty"`
	ToWarehouseID   string                  `json:"to_warehouse_id,omitempty"`
	ContainerType   string                  `json:"container_type"`
	Description     string                  `json:"description"`
	ContentsSummary string                  `json:"contents_summary,omitempty"`
	QuantityUnits   int                     `json:"quantity_units,omitempty"`
	Notes           string                  `json:"notes,omitempty"`
	IdempotencyKey  string                  `json:"idempotency_key"`
	CreatedBy       string                  `json:"created_by"`
	OperatorLabel   string                  `json:"operator_label,omitempty"`
	SessionID       string                  `json:"session_id,omitempty"`
	Lines           []ShippingSlipLineInput `json:"lines,omitempty"`
}

type ShippingSlipLine struct {
	ID          string  `json:"id"`
	SKU         string  `json:"sku,omitempty"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	SortOrder   int     `json:"sort_order"`
}

type ShippingSlip struct {
	ID              string             `json:"id"`
	OrgID           string             `json:"org_id"`
	SlipNumber      string             `json:"slip_number"`
	FromBranchID    string             `json:"from_branch_id"`
	ToBranchID      string             `json:"to_branch_id"`
	FromWarehouseID string             `json:"from_warehouse_id,omitempty"`
	ToWarehouseID   string             `json:"to_warehouse_id,omitempty"`
	ContainerType   string             `json:"container_type"`
	ContainerLabel  string             `json:"container_label,omitempty"`
	Description     string             `json:"description"`
	ContentsSummary string             `json:"contents_summary,omitempty"`
	QuantityUnits   int                `json:"quantity_units"`
	Status          string             `json:"status"`
	PrintedAt       *time.Time         `json:"printed_at,omitempty"`
	ShippedAt       *time.Time         `json:"shipped_at,omitempty"`
	ReceivedAt      *time.Time         `json:"received_at,omitempty"`
	CreatedBy       string             `json:"created_by,omitempty"`
	OperatorLabel   string             `json:"operator_label,omitempty"`
	SessionID       string             `json:"session_id,omitempty"`
	Notes           string             `json:"notes,omitempty"`
	IdempotencyKey  string             `json:"idempotency_key"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	Lines           []ShippingSlipLine `json:"lines,omitempty"`
}

type ShippingSlipFilter struct {
	OrgRef     string
	FromBranch string
	ToBranch   string
	Status     string
	Limit      int
}

const (
	TransportStatusDraft     = "DRAFT"
	TransportStatusPrinted   = "PRINTED"
	TransportStatusInTransit = "IN_TRANSIT"
	TransportStatusDelivered = "DELIVERED"
	TransportStatusCancelled = "CANCELLED"
)

type TransportSectionInput struct {
	DepartmentCode string   `json:"department_code"`
	DepartmentName string   `json:"department_name,omitempty"`
	Notes          string   `json:"notes,omitempty"`
	SlipIDs        []string `json:"slip_ids,omitempty"`
}

type CreateTransportSheetRequest struct {
	OrgID          string                  `json:"org_id"`
	FromBranchID   string                  `json:"from_branch_id"`
	ToBranchID     string                  `json:"to_branch_id"`
	CarrierName    string                  `json:"carrier_name,omitempty"`
	VehicleRef     string                  `json:"vehicle_ref,omitempty"`
	DriverName     string                  `json:"driver_name,omitempty"`
	Notes          string                  `json:"notes,omitempty"`
	IdempotencyKey string                  `json:"idempotency_key"`
	CreatedBy      string                  `json:"created_by"`
	OperatorLabel  string                  `json:"operator_label,omitempty"`
	SessionID      string                  `json:"session_id,omitempty"`
	Sections       []TransportSectionInput `json:"sections"`
}

type TransportLinkedSlip struct {
	ID             string `json:"id"`
	SlipNumber     string `json:"slip_number"`
	ContainerType  string `json:"container_type"`
	ContainerLabel string `json:"container_label,omitempty"`
	Description    string `json:"description"`
	Status         string `json:"status"`
}

type TransportSheetSection struct {
	ID             string                `json:"id"`
	DepartmentCode string                `json:"department_code"`
	DepartmentName string                `json:"department_name"`
	Notes          string                `json:"notes"`
	SortOrder      int                   `json:"sort_order"`
	Slips          []TransportLinkedSlip `json:"slips,omitempty"`
}

type TransportSheet struct {
	ID             string                  `json:"id"`
	OrgID          string                  `json:"org_id"`
	SheetNumber    string                  `json:"sheet_number"`
	FromBranchID   string                  `json:"from_branch_id"`
	ToBranchID     string                  `json:"to_branch_id"`
	CarrierName    string                  `json:"carrier_name,omitempty"`
	VehicleRef     string                  `json:"vehicle_ref,omitempty"`
	DriverName     string                  `json:"driver_name,omitempty"`
	Status         string                  `json:"status"`
	PrintedAt      *time.Time              `json:"printed_at,omitempty"`
	DepartedAt     *time.Time              `json:"departed_at,omitempty"`
	DeliveredAt    *time.Time              `json:"delivered_at,omitempty"`
	CreatedBy      string                  `json:"created_by,omitempty"`
	OperatorLabel  string                  `json:"operator_label,omitempty"`
	SessionID      string                  `json:"session_id,omitempty"`
	Notes          string                  `json:"notes,omitempty"`
	IdempotencyKey string                  `json:"idempotency_key"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
	Sections       []TransportSheetSection `json:"sections,omitempty"`
}

type TransportSheetFilter struct {
	OrgRef     string
	FromBranch string
	ToBranch   string
	Status     string
	Limit      int
}

// Stock transfer between warehouses/branches (atomic TRANSFER_OUT / TRANSFER_IN).
const (
	TransferStatusDraft     = "DRAFT"
	TransferStatusInTransit = "IN_TRANSIT"
	TransferStatusReceived  = "RECEIVED"
	TransferStatusCancelled = "CANCELLED"
)

type TransferLineInput struct {
	SKU      string  `json:"sku"`
	Quantity float64 `json:"quantity"`
}

type CreateTransferRequest struct {
	OrgID           string              `json:"org_id"`
	FromBranchID    string              `json:"from_branch_id"`
	ToBranchID      string              `json:"to_branch_id"`
	FromWarehouseID string              `json:"from_warehouse_id"`
	ToWarehouseID   string              `json:"to_warehouse_id"`
	Notes           string              `json:"notes,omitempty"`
	IdempotencyKey  string              `json:"idempotency_key"`
	CreatedBy       string              `json:"created_by"`
	OperatorLabel   string              `json:"operator_label,omitempty"`
	SessionID       string              `json:"session_id,omitempty"`
	SlipIDs         []string            `json:"slip_ids,omitempty"`
	Lines           []TransferLineInput `json:"lines"`
}

type TransferLine struct {
	ID            string  `json:"id"`
	SKU           string  `json:"sku"`
	Quantity      float64 `json:"quantity"`
	SortOrder     int     `json:"sort_order"`
	OutMovementID string  `json:"out_movement_id,omitempty"`
	InMovementID  string  `json:"in_movement_id,omitempty"`
}

type InventoryTransfer struct {
	ID              string         `json:"id"`
	OrgID           string         `json:"org_id"`
	TransferNumber  string         `json:"transfer_number"`
	FromBranchID    string         `json:"from_branch_id"`
	ToBranchID      string         `json:"to_branch_id"`
	FromWarehouseID string         `json:"from_warehouse_id"`
	ToWarehouseID   string         `json:"to_warehouse_id"`
	Status          string         `json:"status"`
	Notes           string         `json:"notes,omitempty"`
	CreatedBy       string         `json:"created_by,omitempty"`
	OperatorLabel   string         `json:"operator_label,omitempty"`
	ShippedBy       string         `json:"shipped_by,omitempty"`
	ShippedAt       *time.Time     `json:"shipped_at,omitempty"`
	ReceivedBy      string         `json:"received_by,omitempty"`
	ReceivedAt      *time.Time     `json:"received_at,omitempty"`
	CancelledBy     string         `json:"cancelled_by,omitempty"`
	CancelledAt     *time.Time     `json:"cancelled_at,omitempty"`
	CancelReason    string         `json:"cancel_reason,omitempty"`
	IdempotencyKey  string         `json:"idempotency_key"`
	CreatedAt       time.Time      `json:"created_at"`
	SlipIDs         []string       `json:"slip_ids,omitempty"`
	Lines           []TransferLine `json:"lines,omitempty"`
}

type TransferFilter struct {
	OrgRef     string
	FromBranch string
	ToBranch   string
	Status     string
	Limit      int
}

type CancelTransferRequest struct {
	Reason string `json:"reason,omitempty"`
	Actor  string `json:"actor,omitempty"`
}

// Online storefront (public catalog) configuration per branch.
type StorefrontSettings struct {
	ID              string   `json:"id,omitempty"`
	OrgID           string   `json:"org_id,omitempty"`
	BranchID        string   `json:"branch_id"`
	PublicSlug      string   `json:"public_slug"`
	Published       bool     `json:"published"`
	BrandName       string   `json:"brand_name"`
	Tagline         string   `json:"tagline,omitempty"`
	PrimaryColor    string   `json:"primary_color"`
	AccentColor     string   `json:"accent_color"`
	HeroTitle       string   `json:"hero_title"`
	HeroSubtitle    string   `json:"hero_subtitle,omitempty"`
	HeroImageURL    string   `json:"hero_image_url,omitempty"`
	CTALabel        string   `json:"cta_label,omitempty"`
	CTAURL          string   `json:"cta_url,omitempty"`
	ShowPrices      bool     `json:"show_prices"`
	ShowStockBadge  bool     `json:"show_stock_badge"`
	InStockOnly     bool     `json:"in_stock_only"`
	FeaturedSKUs    []string `json:"featured_skus,omitempty"`
	ContactPhone    string   `json:"contact_phone,omitempty"`
	ContactWhatsApp string   `json:"contact_whatsapp,omitempty"`
	ContactEmail    string   `json:"contact_email,omitempty"`
	ContactAddress  string   `json:"contact_address,omitempty"`
	ContactHours    string   `json:"contact_hours,omitempty"`
	MapsURL         string   `json:"maps_url,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
	PublicURL       string   `json:"public_url,omitempty"`
}

type UpsertStorefrontRequest struct {
	OrgID           string   `json:"org_id"`
	BranchID        string   `json:"branch_id"`
	PublicSlug      string   `json:"public_slug"`
	Published       bool     `json:"published"`
	BrandName       string   `json:"brand_name"`
	Tagline         string   `json:"tagline"`
	PrimaryColor    string   `json:"primary_color"`
	AccentColor     string   `json:"accent_color"`
	HeroTitle       string   `json:"hero_title"`
	HeroSubtitle    string   `json:"hero_subtitle"`
	HeroImageURL    string   `json:"hero_image_url"`
	CTALabel        string   `json:"cta_label"`
	CTAURL          string   `json:"cta_url"`
	ShowPrices      bool     `json:"show_prices"`
	ShowStockBadge  bool     `json:"show_stock_badge"`
	InStockOnly     bool     `json:"in_stock_only"`
	FeaturedSKUs    []string `json:"featured_skus"`
	ContactPhone    string   `json:"contact_phone"`
	ContactWhatsApp string   `json:"contact_whatsapp"`
	ContactEmail    string   `json:"contact_email"`
	ContactAddress  string   `json:"contact_address"`
	ContactHours    string   `json:"contact_hours"`
	MapsURL         string   `json:"maps_url"`
	UpdatedBy       string   `json:"updated_by"`
}

type StorefrontCatalogItem struct {
	SKU               string   `json:"sku"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	DepartmentLabel   string   `json:"department_label,omitempty"`
	Brand             string   `json:"brand,omitempty"`
	Currency          string   `json:"currency,omitempty"`
	Price             *float64 `json:"price,omitempty"`
	PriceLabel        string   `json:"price_label,omitempty"`
	InStock           *bool    `json:"in_stock,omitempty"`
	Featured          bool     `json:"featured,omitempty"`
}

type StorefrontPublicView struct {
	Settings StorefrontSettings      `json:"settings"`
	Featured []StorefrontCatalogItem `json:"featured,omitempty"`
	Catalog  []StorefrontCatalogItem `json:"catalog,omitempty"`
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
	CreateShippingSlip(ctx context.Context, req CreateShippingSlipRequest) (ShippingSlip, error)
	ListShippingSlips(ctx context.Context, filter ShippingSlipFilter) ([]ShippingSlip, error)
	GetShippingSlip(ctx context.Context, orgRef, slipID string) (ShippingSlip, error)
	MarkShippingSlipPrinted(ctx context.Context, orgRef, slipID, actor string) (ShippingSlip, error)
	CreateTransportSheet(ctx context.Context, req CreateTransportSheetRequest) (TransportSheet, error)
	ListTransportSheets(ctx context.Context, filter TransportSheetFilter) ([]TransportSheet, error)
	GetTransportSheet(ctx context.Context, orgRef, sheetID string) (TransportSheet, error)
	MarkTransportSheetPrinted(ctx context.Context, orgRef, sheetID, actor string) (TransportSheet, error)
	CreateTransfer(ctx context.Context, req CreateTransferRequest) (InventoryTransfer, error)
	ListTransfers(ctx context.Context, filter TransferFilter) ([]InventoryTransfer, error)
	GetTransfer(ctx context.Context, orgRef, transferID string) (InventoryTransfer, error)
	ShipTransfer(ctx context.Context, orgRef, transferID, actor string) (InventoryTransfer, error)
	ReceiveTransfer(ctx context.Context, orgRef, transferID, actor string) (InventoryTransfer, error)
	CancelTransfer(ctx context.Context, orgRef, transferID string, req CancelTransferRequest) (InventoryTransfer, error)
	GetStorefrontSettings(ctx context.Context, orgRef, branchCode string) (StorefrontSettings, error)
	UpsertStorefrontSettings(ctx context.Context, req UpsertStorefrontRequest) (StorefrontSettings, error)
	GetPublicStorefront(ctx context.Context, slug string) (StorefrontPublicView, error)
}
