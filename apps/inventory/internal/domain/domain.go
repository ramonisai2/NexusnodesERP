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

// Shrink / adjustment reason codes (merma, robo, etc.).
const (
	ReasonMerma         = "MERMA"
	ReasonRobo          = "ROBO"
	ReasonDamage        = "DAMAGE"
	ReasonExpired       = "EXPIRED"
	ReasonCountVariance = "COUNT_VARIANCE"
	ReasonFound         = "FOUND"
	ReasonOther         = "OTHER"
)

type MovementRequest struct {
	OrgID           string  `json:"org_id"`
	BranchID        string  `json:"branch_id"`
	WarehouseID     string  `json:"warehouse_id"`
	SKUID           string  `json:"sku_id"`
	MovementType    string  `json:"movement_type"`
	Quantity        float64 `json:"quantity"`
	ReasonCode      string  `json:"reason_code,omitempty"`
	Notes           string  `json:"notes,omitempty"`
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
	ReasonCode     string     `json:"reason_code,omitempty"`
	Notes          string     `json:"notes,omitempty"`
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
	ReasonCode  string
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

// —— CEDI truck / tarimas / cajas ——

const (
	ShipmentReceiving = "RECEIVING"
	ShipmentPosted    = "POSTED"
	ShipmentCancelled = "CANCELLED"
)

type InboundBoxInput struct {
	SKU         string   `json:"sku"`
	BoxesCount  int      `json:"boxes_count"`
	UnitsPerBox float64  `json:"units_per_box"`
	UnitCost    *float64 `json:"unit_cost,omitempty"`
	Description string   `json:"description,omitempty"`
	BoxCode     string   `json:"box_code,omitempty"`
}

type InboundPalletInput struct {
	PalletNo int               `json:"pallet_no,omitempty"`
	Label    string            `json:"label,omitempty"`
	Notes    string            `json:"notes,omitempty"`
	Boxes    []InboundBoxInput `json:"boxes"`
}

type CreateInboundShipmentRequest struct {
	OrgID           string               `json:"org_id"`
	BranchID        string               `json:"branch_id"`
	WarehouseID     string               `json:"warehouse_id"`
	SupplierName    string               `json:"supplier_name"`
	InvoiceNumber   string               `json:"invoice_number"`
	InvoiceDate     string               `json:"invoice_date,omitempty"`
	CarrierName     string               `json:"carrier_name,omitempty"`
	VehicleRef      string               `json:"vehicle_ref,omitempty"`
	DriverName      string               `json:"driver_name,omitempty"`
	DockDoor        string               `json:"dock_door,omitempty"`
	ExpectedPallets int                  `json:"expected_pallets,omitempty"`
	Notes           string               `json:"notes,omitempty"`
	PrintLabels     *bool                `json:"print_labels,omitempty"`
	IdempotencyKey  string               `json:"idempotency_key"`
	CreatedBy       string               `json:"created_by,omitempty"`
	OperatorLabel   string               `json:"operator_label,omitempty"`
	SessionID       string               `json:"session_id,omitempty"`
	Pallets         []InboundPalletInput `json:"pallets"`
}

type InboundBox struct {
	ID          string   `json:"id"`
	BoxNo       int      `json:"box_no"`
	BoxCode     string   `json:"box_code,omitempty"`
	SKU         string   `json:"sku"`
	Description string   `json:"description,omitempty"`
	BoxesCount  int      `json:"boxes_count"`
	UnitsPerBox float64  `json:"units_per_box"`
	Quantity    float64  `json:"quantity"`
	UnitCost    *float64 `json:"unit_cost,omitempty"`
}

type InboundPallet struct {
	ID         string       `json:"id"`
	PalletNo   int          `json:"pallet_no"`
	PalletCode string       `json:"pallet_code"`
	Label      string       `json:"label,omitempty"`
	Status     string       `json:"status"`
	Notes      string       `json:"notes,omitempty"`
	Boxes      []InboundBox `json:"boxes,omitempty"`
	BoxCount   int          `json:"box_count"`
	UnitTotal  float64      `json:"unit_total"`
}

type InboundShipment struct {
	ID              string          `json:"id"`
	OrgID           string          `json:"org_id"`
	BranchID        string          `json:"branch_id"`
	WarehouseID     string          `json:"warehouse_id"`
	WarehouseKind   string          `json:"warehouse_kind,omitempty"`
	ReceiptID       string          `json:"receipt_id,omitempty"`
	SupplierName    string          `json:"supplier_name"`
	InvoiceNumber   string          `json:"invoice_number"`
	InvoiceDate     *string         `json:"invoice_date,omitempty"`
	CarrierName     string          `json:"carrier_name,omitempty"`
	VehicleRef      string          `json:"vehicle_ref,omitempty"`
	DriverName      string          `json:"driver_name,omitempty"`
	DockDoor        string          `json:"dock_door,omitempty"`
	ExpectedPallets int             `json:"expected_pallets"`
	Notes           string          `json:"notes,omitempty"`
	Status          string          `json:"status"`
	ArrivedAt       *time.Time      `json:"arrived_at,omitempty"`
	PostedAt        *time.Time      `json:"posted_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	Pallets         []InboundPallet `json:"pallets,omitempty"`
	PalletCount     int             `json:"pallet_count"`
	BoxCount        int             `json:"box_count"`
	UnitTotal       float64         `json:"unit_total"`
	SKUSummary      []ReceiptLine   `json:"sku_summary,omitempty"`
	Receipt         *Receipt        `json:"receipt,omitempty"`
}

type InboundShipmentFilter struct {
	OrgRef     string
	BranchCode string
	Status     string
	Limit      int
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

// ParcelKind classifies inter-site packages (stores, CEDI, defective, warranty).
const (
	ParcelTransfer         = "TRANSFER"
	ParcelCEDIDistribution = "CEDI_DISTRIBUTION"
	ParcelDefective        = "DEFECTIVE"
	ParcelWarranty         = "WARRANTY"
	ParcelReturnToCEDI     = "RETURN_TO_CEDI"
	ParcelReturnToVendor   = "RETURN_TO_VENDOR"
	ParcelRepairOut        = "REPAIR_OUT"
	ParcelRepairIn         = "REPAIR_IN"
)

func ValidParcelKind(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case ParcelTransfer, ParcelCEDIDistribution, ParcelDefective, ParcelWarranty,
		ParcelReturnToCEDI, ParcelReturnToVendor, ParcelRepairOut, ParcelRepairIn:
		return true
	default:
		return false
	}
}

func ParcelKindLabelES(code string) string {
	switch strings.ToUpper(code) {
	case ParcelTransfer:
		return "Traslado"
	case ParcelCEDIDistribution:
		return "Distribución CEDI"
	case ParcelDefective:
		return "Defectuoso"
	case ParcelWarranty:
		return "Garantía"
	case ParcelReturnToCEDI:
		return "Devolución a CEDI"
	case ParcelReturnToVendor:
		return "Devolución a proveedor"
	case ParcelRepairOut:
		return "Envío a taller"
	case ParcelRepairIn:
		return "Regreso de taller"
	default:
		return code
	}
}

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
	ParcelKind      string                  `json:"parcel_kind,omitempty"`
	TrackingCode    string                  `json:"tracking_code,omitempty"`
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
	ParcelKind      string             `json:"parcel_kind"`
	ParcelKindLabel string             `json:"parcel_kind_label,omitempty"`
	TrackingCode    string             `json:"tracking_code,omitempty"`
	Description     string             `json:"description"`
	ContentsSummary string             `json:"contents_summary,omitempty"`
	QuantityUnits   int                `json:"quantity_units"`
	Status          string             `json:"status"`
	PrintedAt       *time.Time         `json:"printed_at,omitempty"`
	ShippedAt       *time.Time         `json:"shipped_at,omitempty"`
	ReceivedAt      *time.Time         `json:"received_at,omitempty"`
	CancelledAt     *time.Time         `json:"cancelled_at,omitempty"`
	CancelReason    string             `json:"cancel_reason,omitempty"`
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
	ParcelKind string
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
	ParcelKind     string                  `json:"parcel_kind,omitempty"`
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
	ParcelKind     string                  `json:"parcel_kind"`
	ParcelKindLabel string                 `json:"parcel_kind_label,omitempty"`
	Status         string                  `json:"status"`
	PrintedAt      *time.Time              `json:"printed_at,omitempty"`
	DepartedAt     *time.Time              `json:"departed_at,omitempty"`
	DeliveredAt    *time.Time              `json:"delivered_at,omitempty"`
	CancelledAt    *time.Time              `json:"cancelled_at,omitempty"`
	CancelReason   string                  `json:"cancel_reason,omitempty"`
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
	ParcelKind string
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
	OrgID             string              `json:"org_id"`
	FromBranchID      string              `json:"from_branch_id"`
	ToBranchID        string              `json:"to_branch_id"`
	FromWarehouseID   string              `json:"from_warehouse_id"`
	ToWarehouseID     string              `json:"to_warehouse_id"`
	ParcelKind        string              `json:"parcel_kind,omitempty"`
	TransportSheetID  string              `json:"transport_sheet_id,omitempty"`
	WarrantyCaseID    string              `json:"warranty_case_id,omitempty"`
	ReturnCaseID      string              `json:"return_case_id,omitempty"`
	Notes             string              `json:"notes,omitempty"`
	IdempotencyKey    string              `json:"idempotency_key"`
	CreatedBy         string              `json:"created_by"`
	OperatorLabel     string              `json:"operator_label,omitempty"`
	SessionID         string              `json:"session_id,omitempty"`
	SlipIDs           []string            `json:"slip_ids,omitempty"`
	Lines             []TransferLineInput `json:"lines"`
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
	ID               string         `json:"id"`
	OrgID            string         `json:"org_id"`
	TransferNumber   string         `json:"transfer_number"`
	FromBranchID     string         `json:"from_branch_id"`
	ToBranchID       string         `json:"to_branch_id"`
	FromWarehouseID  string         `json:"from_warehouse_id"`
	ToWarehouseID    string         `json:"to_warehouse_id"`
	ParcelKind       string         `json:"parcel_kind"`
	ParcelKindLabel  string         `json:"parcel_kind_label,omitempty"`
	TransportSheetID string         `json:"transport_sheet_id,omitempty"`
	WarrantyCaseID   string         `json:"warranty_case_id,omitempty"`
	ReturnCaseID     string         `json:"return_case_id,omitempty"`
	Status           string         `json:"status"`
	Notes            string         `json:"notes,omitempty"`
	CreatedBy        string         `json:"created_by,omitempty"`
	OperatorLabel    string         `json:"operator_label,omitempty"`
	ShippedBy        string         `json:"shipped_by,omitempty"`
	ShippedAt        *time.Time     `json:"shipped_at,omitempty"`
	ReceivedBy       string         `json:"received_by,omitempty"`
	ReceivedAt       *time.Time     `json:"received_at,omitempty"`
	CancelledBy      string         `json:"cancelled_by,omitempty"`
	CancelledAt      *time.Time     `json:"cancelled_at,omitempty"`
	CancelReason     string         `json:"cancel_reason,omitempty"`
	IdempotencyKey   string         `json:"idempotency_key"`
	CreatedAt        time.Time      `json:"created_at"`
	SlipIDs          []string       `json:"slip_ids,omitempty"`
	Lines            []TransferLine `json:"lines,omitempty"`
}

type TransferFilter struct {
	OrgRef     string
	FromBranch string
	ToBranch   string
	Status     string
	ParcelKind string
	Limit      int
}

type CancelTransferRequest struct {
	Reason string `json:"reason,omitempty"`
	Actor  string `json:"actor,omitempty"`
}

type CancelParcelRequest struct {
	Reason string `json:"reason,omitempty"`
	Actor  string `json:"actor,omitempty"`
}

const (
	WarrantyStatusOpen     = "OPEN"
	WarrantyStatusShipped  = "SHIPPED"
	WarrantyStatusReceived = "RECEIVED"
	WarrantyStatusInRepair = "IN_REPAIR"
	WarrantyStatusClosed   = "CLOSED"
	WarrantyStatusCancelled = "CANCELLED"
)

type CreateWarrantyCaseRequest struct {
	OrgID               string `json:"org_id"`
	BranchID            string `json:"branch_id"`
	DestinationBranchID string `json:"destination_branch_id,omitempty"`
	SKU                 string `json:"sku,omitempty"`
	SerialNumber        string `json:"serial_number,omitempty"`
	CustomerRef         string `json:"customer_ref,omitempty"`
	ProblemDescription  string `json:"problem_description"`
	IdempotencyKey      string `json:"idempotency_key"`
	CreatedBy           string `json:"created_by"`
	OperatorLabel       string `json:"operator_label,omitempty"`
}

type WarrantyCase struct {
	ID                  string     `json:"id"`
	OrgID               string     `json:"org_id"`
	CaseNumber          string     `json:"case_number"`
	BranchID            string     `json:"branch_id"`
	DestinationBranchID string     `json:"destination_branch_id,omitempty"`
	SKU                 string     `json:"sku,omitempty"`
	SerialNumber        string     `json:"serial_number,omitempty"`
	CustomerRef         string     `json:"customer_ref,omitempty"`
	ProblemDescription  string     `json:"problem_description"`
	Status              string     `json:"status"`
	ParcelKind          string     `json:"parcel_kind"`
	ShippingSlipID      string     `json:"shipping_slip_id,omitempty"`
	TransferID          string     `json:"transfer_id,omitempty"`
	CreatedBy           string     `json:"created_by,omitempty"`
	OperatorLabel       string     `json:"operator_label,omitempty"`
	ClosedAt            *time.Time `json:"closed_at,omitempty"`
	IdempotencyKey      string     `json:"idempotency_key"`
	CreatedAt           time.Time  `json:"created_at"`
}

type WarrantyCaseFilter struct {
	OrgRef   string
	BranchID string
	Status   string
	Limit    int
}

const (
	ReturnReasonDefective      = "DEFECTIVE"
	ReturnReasonWarranty       = "WARRANTY"
	ReturnReasonCustomerReturn = "CUSTOMER_RETURN"
	ReturnReasonOverstock      = "OVERSTOCK"
	ReturnReasonOther          = "OTHER"
)

type CreateReturnCaseRequest struct {
	OrgID          string `json:"org_id"`
	FromBranchID   string `json:"from_branch_id"`
	ToBranchID     string `json:"to_branch_id"`
	Reason         string `json:"reason"`
	Notes          string `json:"notes,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
	CreatedBy      string `json:"created_by"`
	OperatorLabel  string `json:"operator_label,omitempty"`
}

type ReturnCase struct {
	ID             string     `json:"id"`
	OrgID          string     `json:"org_id"`
	CaseNumber     string     `json:"case_number"`
	FromBranchID   string     `json:"from_branch_id"`
	ToBranchID     string     `json:"to_branch_id"`
	Reason         string     `json:"reason"`
	Notes          string     `json:"notes,omitempty"`
	Status         string     `json:"status"`
	ParcelKind     string     `json:"parcel_kind"`
	ShippingSlipID string     `json:"shipping_slip_id,omitempty"`
	TransferID     string     `json:"transfer_id,omitempty"`
	CreatedBy      string     `json:"created_by,omitempty"`
	OperatorLabel  string     `json:"operator_label,omitempty"`
	ClosedAt       *time.Time `json:"closed_at,omitempty"`
	IdempotencyKey string     `json:"idempotency_key"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ReturnCaseFilter struct {
	OrgRef     string
	FromBranch string
	Status     string
	Limit      int
}

type ParcelHubItem struct {
	Kind            string `json:"kind"` // slip | transfer | transport | warranty | return
	ID              string `json:"id"`
	Number          string `json:"number"`
	ParcelKind      string `json:"parcel_kind"`
	ParcelKindLabel string `json:"parcel_kind_label,omitempty"`
	Status          string `json:"status"`
	FromBranchID    string `json:"from_branch_id,omitempty"`
	ToBranchID      string `json:"to_branch_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type ParcelHubFilter struct {
	OrgRef     string
	BranchID   string
	ParcelKind string
	Limit      int
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

// Customer accounts + loyalty/membership cards (barcode or chip).
const (
	CustomerStatusActive    = "ACTIVE"
	CustomerStatusSuspended = "SUSPENDED"
	CustomerStatusClosed    = "CLOSED"

	CardKindBarcode = "BARCODE"
	CardKindChip    = "CHIP"

	CardStatusActive  = "ACTIVE"
	CardStatusBlocked = "BLOCKED"
	CardStatusLost    = "LOST"
	CardStatusExpired = "EXPIRED"
)

func ValidCardKind(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case CardKindBarcode, CardKindChip:
		return true
	default:
		return false
	}
}

func CardKindLabelES(code string) string {
	switch strings.ToUpper(code) {
	case CardKindBarcode:
		return "Código de barras"
	case CardKindChip:
		return "Chip / NFC"
	default:
		return code
	}
}

type RegisterCustomerRequest struct {
	StorefrontSlug string `json:"storefront_slug"`
	OrgID          string `json:"org_id,omitempty"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	DisplayName    string `json:"display_name"`
	Phone          string `json:"phone,omitempty"`
	PreferredBranch string `json:"preferred_branch_id,omitempty"`
}

type LoginCustomerRequest struct {
	StorefrontSlug string `json:"storefront_slug"`
	OrgID          string `json:"org_id,omitempty"`
	Email          string `json:"email"`
	Password       string `json:"password"`
}

type Customer struct {
	ID                string    `json:"id"`
	OrgID             string    `json:"org_id"`
	Email             string    `json:"email"`
	Phone             string    `json:"phone,omitempty"`
	DisplayName       string    `json:"display_name"`
	Status            string    `json:"status"`
	PreferredBranchID string    `json:"preferred_branch_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	Cards             []CustomerCard `json:"cards,omitempty"`
}

type CustomerFilter struct {
	OrgRef string
	Query  string
	Status string
	Limit  int
}

type CreateCustomerCardRequest struct {
	OrgID      string `json:"org_id"`
	CustomerID string `json:"customer_id"`
	CardKind   string `json:"card_kind"`
	CardCode   string `json:"card_code,omitempty"`
	Label      string `json:"label,omitempty"`
	IssuedBy   string `json:"issued_by,omitempty"`
}

type CustomerCard struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	CustomerID   string     `json:"customer_id"`
	CardKind     string     `json:"card_kind"`
	CardKindLabel string    `json:"card_kind_label,omitempty"`
	CardCode     string     `json:"card_code"`
	Label        string     `json:"label,omitempty"`
	Status       string     `json:"status"`
	IssuedBy     string     `json:"issued_by,omitempty"`
	BlockedReason string    `json:"blocked_reason,omitempty"`
	BlockedAt    *time.Time `json:"blocked_at,omitempty"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type BlockCardRequest struct {
	Reason string `json:"reason,omitempty"`
	Actor  string `json:"actor,omitempty"`
}

type CardLookupResult struct {
	Card     CustomerCard `json:"card"`
	Customer Customer     `json:"customer"`
}

// —— POS / Caja ——

const (
	PaymentCash     = "CASH"
	PaymentCard     = "CARD"
	PaymentTransfer = "TRANSFER"
	PaymentOther    = "OTHER"
	SaleCompleted   = "COMPLETED"
	SaleVoid        = "VOID"
	InvoiceRequested = "REQUESTED"
)

type BranchFiscalSettings struct {
	BranchID         string  `json:"branch_id"`
	OrgID            string  `json:"org_id"`
	LegalName        string  `json:"legal_name"`
	TradeName        string  `json:"trade_name"`
	RFC              string  `json:"rfc"`
	TaxRegime        string  `json:"tax_regime"`
	PostalCode       string  `json:"postal_code"`
	PricesIncludeTax bool    `json:"prices_include_tax"`
	DefaultTaxRate   float64 `json:"default_tax_rate"`
	TicketSeries     string  `json:"ticket_series"`
	NextTicketFolio  int64   `json:"next_ticket_folio,omitempty"`
	InvoiceSeries    string  `json:"invoice_series"`
	ReceiptFooter    string  `json:"receipt_footer"`
}

type UpsertFiscalSettingsRequest struct {
	OrgID            string  `json:"org_id"`
	BranchID         string  `json:"branch_id"`
	LegalName        string  `json:"legal_name"`
	TradeName        string  `json:"trade_name"`
	RFC              string  `json:"rfc"`
	TaxRegime        string  `json:"tax_regime"`
	PostalCode       string  `json:"postal_code"`
	PricesIncludeTax *bool   `json:"prices_include_tax,omitempty"`
	DefaultTaxRate   *float64 `json:"default_tax_rate,omitempty"`
	TicketSeries     string  `json:"ticket_series"`
	InvoiceSeries    string  `json:"invoice_series"`
	ReceiptFooter    string  `json:"receipt_footer"`
}

type POSPaymentInput struct {
	Method          string   `json:"method"`
	Amount          float64  `json:"amount"`
	ReceivedAmount  *float64 `json:"received_amount,omitempty"`
	Reference       string   `json:"reference,omitempty"`
}

type POSLineInput struct {
	SKU      string  `json:"sku"`
	Quantity float64 `json:"quantity"`
}

type CompleteSaleRequest struct {
	OrgID           string            `json:"org_id"`
	BranchID        string            `json:"branch_id"`
	WarehouseID     string            `json:"warehouse_id,omitempty"`
	CustomerID      string            `json:"customer_id,omitempty"`
	CustomerName    string            `json:"customer_name,omitempty"`
	Lines           []POSLineInput    `json:"lines"`
	Payments        []POSPaymentInput `json:"payments"`
	RequestInvoice  bool              `json:"request_invoice"`
	InvoiceRFC      string            `json:"invoice_rfc,omitempty"`
	InvoiceName     string            `json:"invoice_name,omitempty"`
	InvoiceRegime   string            `json:"invoice_tax_regime,omitempty"`
	InvoicePostal   string            `json:"invoice_postal_code,omitempty"`
	InvoiceUsoCFDI  string            `json:"invoice_uso_cfdi,omitempty"`
	InvoiceEmail    string            `json:"invoice_email,omitempty"`
	Notes           string            `json:"notes,omitempty"`
	IdempotencyKey  string            `json:"idempotency_key"`
	CashierSub      string            `json:"cashier_sub,omitempty"`
	OperatorLabel   string            `json:"operator_label,omitempty"`
	SessionID       string            `json:"session_id,omitempty"`
	StationID       string            `json:"station_id,omitempty"`
}

type POSSaleLine struct {
	ID          string  `json:"id"`
	LineNo      int     `json:"line_no"`
	SKU         string  `json:"sku"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	LineTotal   float64 `json:"line_total"`
	TaxRate     float64 `json:"tax_rate"`
	TaxAmount   float64 `json:"tax_amount"`
	BaseAmount  float64 `json:"base_amount"`
	MovementID  string  `json:"movement_id,omitempty"`
}

type POSPayment struct {
	ID             string  `json:"id"`
	Method         string  `json:"method"`
	MethodLabel    string  `json:"method_label,omitempty"`
	Amount         float64 `json:"amount"`
	ReceivedAmount *float64 `json:"received_amount,omitempty"`
	ChangeAmount   float64 `json:"change_amount"`
	Reference      string  `json:"reference,omitempty"`
}

type FiscalInvoice struct {
	ID                 string     `json:"id"`
	SaleID             string     `json:"sale_id"`
	Status             string     `json:"status"`
	StatusLabel        string     `json:"status_label,omitempty"`
	Series             string     `json:"series"`
	Folio              *int64     `json:"folio,omitempty"`
	UUID               string     `json:"uuid,omitempty"`
	RFCReceiver        string     `json:"rfc_receiver"`
	LegalNameReceiver  string     `json:"legal_name_receiver"`
	TaxRegimeReceiver  string     `json:"tax_regime_receiver"`
	PostalCodeReceiver string     `json:"postal_code_receiver"`
	UsoCFDI            string     `json:"uso_cfdi"`
	EmailCFDI          string     `json:"email_cfdi,omitempty"`
	Subtotal           float64    `json:"subtotal"`
	TaxTotal           float64    `json:"tax_total"`
	GrandTotal         float64    `json:"grand_total"`
	Notes              string     `json:"notes,omitempty"`
	RequestedAt        time.Time  `json:"requested_at"`
	StampedAt          *time.Time `json:"stamped_at,omitempty"`
}

type POSSale struct {
	ID               string          `json:"id"`
	OrgID            string          `json:"org_id"`
	BranchID         string          `json:"branch_id"`
	WarehouseID      string          `json:"warehouse_id,omitempty"`
	TicketNumber     string          `json:"ticket_number"`
	Status           string          `json:"status"`
	Currency         string          `json:"currency"`
	PricesIncludeTax bool            `json:"prices_include_tax"`
	TaxRate          float64         `json:"tax_rate"`
	Subtotal         float64         `json:"subtotal"`
	TaxTotal         float64         `json:"tax_total"`
	DiscountTotal    float64         `json:"discount_total"`
	GrandTotal       float64         `json:"grand_total"`
	CustomerID       string          `json:"customer_id,omitempty"`
	CustomerName     string          `json:"customer_name,omitempty"`
	CashierSub       string          `json:"cashier_sub,omitempty"`
	OperatorLabel    string          `json:"operator_label,omitempty"`
	SessionID        string          `json:"session_id,omitempty"`
	StationID        string          `json:"station_id,omitempty"`
	Notes            string          `json:"notes,omitempty"`
	RequestInvoice   bool            `json:"request_invoice"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	Lines            []POSSaleLine   `json:"lines,omitempty"`
	Payments         []POSPayment    `json:"payments,omitempty"`
	Invoice          *FiscalInvoice  `json:"invoice,omitempty"`
	Fiscal           *BranchFiscalSettings `json:"fiscal,omitempty"`
	ReceiptHint      string          `json:"receipt_hint,omitempty"`
}

type SaleFilter struct {
	OrgRef     string
	BranchCode string
	Limit      int
}

type RequestInvoiceRequest struct {
	RFC        string `json:"rfc"`
	LegalName  string `json:"legal_name"`
	TaxRegime  string `json:"tax_regime,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	UsoCFDI    string `json:"uso_cfdi,omitempty"`
	Email      string `json:"email,omitempty"`
	Actor      string `json:"actor,omitempty"`
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
	ShipShippingSlip(ctx context.Context, orgRef, slipID, actor string) (ShippingSlip, error)
	ReceiveShippingSlip(ctx context.Context, orgRef, slipID, actor string) (ShippingSlip, error)
	CancelShippingSlip(ctx context.Context, orgRef, slipID string, req CancelParcelRequest) (ShippingSlip, error)
	CreateTransportSheet(ctx context.Context, req CreateTransportSheetRequest) (TransportSheet, error)
	ListTransportSheets(ctx context.Context, filter TransportSheetFilter) ([]TransportSheet, error)
	GetTransportSheet(ctx context.Context, orgRef, sheetID string) (TransportSheet, error)
	MarkTransportSheetPrinted(ctx context.Context, orgRef, sheetID, actor string) (TransportSheet, error)
	DepartTransportSheet(ctx context.Context, orgRef, sheetID, actor string) (TransportSheet, error)
	DeliverTransportSheet(ctx context.Context, orgRef, sheetID, actor string) (TransportSheet, error)
	CancelTransportSheet(ctx context.Context, orgRef, sheetID string, req CancelParcelRequest) (TransportSheet, error)
	CreateTransfer(ctx context.Context, req CreateTransferRequest) (InventoryTransfer, error)
	ListTransfers(ctx context.Context, filter TransferFilter) ([]InventoryTransfer, error)
	GetTransfer(ctx context.Context, orgRef, transferID string) (InventoryTransfer, error)
	ShipTransfer(ctx context.Context, orgRef, transferID, actor string) (InventoryTransfer, error)
	ReceiveTransfer(ctx context.Context, orgRef, transferID, actor string) (InventoryTransfer, error)
	CancelTransfer(ctx context.Context, orgRef, transferID string, req CancelTransferRequest) (InventoryTransfer, error)
	CreateWarrantyCase(ctx context.Context, req CreateWarrantyCaseRequest) (WarrantyCase, error)
	ListWarrantyCases(ctx context.Context, filter WarrantyCaseFilter) ([]WarrantyCase, error)
	GetWarrantyCase(ctx context.Context, orgRef, caseID string) (WarrantyCase, error)
	CreateReturnCase(ctx context.Context, req CreateReturnCaseRequest) (ReturnCase, error)
	ListReturnCases(ctx context.Context, filter ReturnCaseFilter) ([]ReturnCase, error)
	GetReturnCase(ctx context.Context, orgRef, caseID string) (ReturnCase, error)
	ListParcelHub(ctx context.Context, filter ParcelHubFilter) ([]ParcelHubItem, error)
	GetStorefrontSettings(ctx context.Context, orgRef, branchCode string) (StorefrontSettings, error)
	UpsertStorefrontSettings(ctx context.Context, req UpsertStorefrontRequest) (StorefrontSettings, error)
	GetPublicStorefront(ctx context.Context, slug string) (StorefrontPublicView, error)
	RegisterCustomer(ctx context.Context, req RegisterCustomerRequest) (Customer, error)
	AuthenticateCustomer(ctx context.Context, req LoginCustomerRequest) (Customer, error)
	GetCustomer(ctx context.Context, orgRef, customerID string) (Customer, error)
	ListCustomers(ctx context.Context, filter CustomerFilter) ([]Customer, error)
	ListCustomerCards(ctx context.Context, orgRef, customerID string) ([]CustomerCard, error)
	CreateCustomerCard(ctx context.Context, req CreateCustomerCardRequest) (CustomerCard, error)
	BlockCustomerCard(ctx context.Context, orgRef, cardID string, req BlockCardRequest) (CustomerCard, error)
	LookupCustomerCard(ctx context.Context, orgRef, cardCode string) (CardLookupResult, error)
	ResolveOrgByStorefrontSlug(ctx context.Context, slug string) (orgID, branchCode string, err error)
	GetFiscalSettings(ctx context.Context, orgRef, branchCode string) (BranchFiscalSettings, error)
	UpsertFiscalSettings(ctx context.Context, req UpsertFiscalSettingsRequest) (BranchFiscalSettings, error)
	CompleteSale(ctx context.Context, req CompleteSaleRequest) (POSSale, error)
	GetSale(ctx context.Context, orgRef, saleID string) (POSSale, error)
	ListSales(ctx context.Context, filter SaleFilter) ([]POSSale, error)
	RequestSaleInvoice(ctx context.Context, orgRef, saleID string, req RequestInvoiceRequest) (FiscalInvoice, error)
	CreateInboundShipment(ctx context.Context, req CreateInboundShipmentRequest) (InboundShipment, error)
	GetInboundShipment(ctx context.Context, orgRef, shipmentID string) (InboundShipment, error)
	ListInboundShipments(ctx context.Context, filter InboundShipmentFilter) ([]InboundShipment, error)
	PostInboundShipment(ctx context.Context, orgRef, shipmentID, postedBy string) (InboundShipment, error)
}
