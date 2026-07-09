package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
)

type memPlacement struct {
	productSKU string
	branchID   string
	deptCode   string
	deptName   string
	catCode    string
	catName    string
	isPrimary  bool
}

type Memory struct {
	mu            sync.Mutex
	balances      map[string]*domain.StockBalance
	movements     map[string]domain.Movement
	movementsByID map[string]domain.Movement
	departments   []domain.Department
	placements    []memPlacement
	catalog       []domain.CatalogItem
	labels        []domain.StoreLabel
}

func NewMemory() *Memory {
	return &Memory{
		balances:      map[string]*domain.StockBalance{},
		movements:     map[string]domain.Movement{},
		movementsByID: map[string]domain.Movement{},
	}
}

func f64(v float64) *float64 { return &v }

func (s *Memory) SeedDemo() {
	items := []domain.StockBalance{
		{ID: "bal_1", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "BOLT-M8", SKU: "BOLT-M8", ProductName: "Tornillo M8", OnHand: 1000, Version: 1},
		{ID: "bal_2", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "NUT-M8", SKU: "NUT-M8", ProductName: "Tuerca M8", OnHand: 800, Version: 1},
		{ID: "bal_3", WarehouseID: "wh_sur", BranchID: "br_sur", SKUID: "BOLT-M8", SKU: "BOLT-M8", ProductName: "Tornillo M8", OnHand: 400, Version: 1},
		{
			ID: "bal_4", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "LAPTOP-14", SKU: "LAPTOP-14",
			ProductName: `Laptop 14"`, OnHand: 25, Version: 1,
			Departments: []string{"electronica"}, Categories: []string{"computo"},
			Placements: []string{"Electrónica / Cómputo"},
		},
		{
			ID: "bal_5", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "PHONE-X", SKU: "PHONE-X",
			ProductName: "Smartphone X", OnHand: 40, Version: 1,
			Departments: []string{"electronica"}, Categories: []string{"telefonia"},
			Placements: []string{"Electrónica / Telefonía"},
		},
		{
			ID: "bal_6", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "FIG-COL-01", SKU: "FIG-COL-01",
			ProductName: "Figura coleccionable ed. limitada", OnHand: 15, Version: 1,
			Departments: []string{"electronica", "jugueteria"}, Categories: []string{"videojuegos", "coleccionables"},
			Placements: []string{"Electrónica / Videojuegos", "Juguetería / Coleccionables"},
		},
		{
			ID: "bal_7", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "CAAL686101YL1", SKU: "CAAL686101YL1",
			ProductName: "Camiseta manga corta de hombre Club América", OnHand: 48, Version: 1,
			Departments: []string{"ropa_deportiva"}, Categories: []string{"playeras"},
			Placements: []string{"Ropa Deportiva / Playeras"},
		},
	}
	for i := range items {
		b := items[i]
		key := b.WarehouseID + "|" + b.SKUID
		cp := b
		s.balances[key] = &cp
	}

	s.departments = []domain.Department{
		{
			Code: "electronica", Name: "Electrónica", BranchID: "br_norte", SortOrder: 10,
			Categories: []domain.DepartmentCategory{
				{Code: "computo", Name: "Cómputo", SortOrder: 10},
				{Code: "video", Name: "Video", SortOrder: 20},
				{Code: "telefonia", Name: "Telefonía", SortOrder: 30},
				{Code: "videojuegos", Name: "Videojuegos", SortOrder: 40},
			},
		},
		{
			Code: "jugueteria", Name: "Juguetería", BranchID: "br_norte", SortOrder: 20,
			Categories: []domain.DepartmentCategory{
				{Code: "coleccionables", Name: "Coleccionables", SortOrder: 10},
				{Code: "juegos_mesa", Name: "Juegos de mesa", SortOrder: 20},
			},
		},
		{
			Code: "ropa_deportiva", Name: "Ropa Deportiva", BranchID: "br_norte", SortOrder: 30,
			Categories: []domain.DepartmentCategory{
				{Code: "playeras", Name: "Playeras", SortOrder: 10},
			},
		},
	}

	s.placements = []memPlacement{
		{productSKU: "LAPTOP-14", branchID: "br_norte", deptCode: "electronica", deptName: "Electrónica", catCode: "computo", catName: "Cómputo", isPrimary: true},
		{productSKU: "PHONE-X", branchID: "br_norte", deptCode: "electronica", deptName: "Electrónica", catCode: "telefonia", catName: "Telefonía", isPrimary: true},
		{productSKU: "FIG-COL-01", branchID: "br_norte", deptCode: "electronica", deptName: "Electrónica", catCode: "videojuegos", catName: "Videojuegos", isPrimary: true},
		{productSKU: "FIG-COL-01", branchID: "br_norte", deptCode: "jugueteria", deptName: "Juguetería", catCode: "coleccionables", catName: "Coleccionables", isPrimary: false},
		{productSKU: "CAAL686101YL1", branchID: "br_norte", deptCode: "ropa_deportiva", deptName: "Ropa Deportiva", catCode: "playeras", catName: "Playeras", isPrimary: true},
	}

	s.catalog = []domain.CatalogItem{
		{
			ProductID: "p_laptop", SKUBase: "LAPTOP", Name: `Laptop 14"`, SKUs: []string{"LAPTOP-14"},
			Placements: []domain.CatalogPlacement{
				{DepartmentCode: "electronica", DepartmentName: "Electrónica", CategoryCode: "computo", CategoryName: "Cómputo", IsPrimary: true},
			},
		},
		{
			ProductID: "p_phone", SKUBase: "PHONE", Name: "Smartphone X", SKUs: []string{"PHONE-X"},
			Placements: []domain.CatalogPlacement{
				{DepartmentCode: "electronica", DepartmentName: "Electrónica", CategoryCode: "telefonia", CategoryName: "Telefonía", IsPrimary: true},
			},
		},
		{
			ProductID: "p_fig", SKUBase: "FIG-COL", Name: "Figura coleccionable ed. limitada", SKUs: []string{"FIG-COL-01"},
			Placements: []domain.CatalogPlacement{
				{DepartmentCode: "electronica", DepartmentName: "Electrónica", CategoryCode: "videojuegos", CategoryName: "Videojuegos", IsPrimary: true},
				{DepartmentCode: "jugueteria", DepartmentName: "Juguetería", CategoryCode: "coleccionables", CategoryName: "Coleccionables", IsPrimary: false},
			},
		},
		{
			ProductID: "p_jersey", SKUBase: "CAAL686", Name: "Camiseta manga corta de hombre Club América", SKUs: []string{"CAAL686101YL1"},
			Placements: []domain.CatalogPlacement{
				{DepartmentCode: "ropa_deportiva", DepartmentName: "Ropa Deportiva", CategoryCode: "playeras", CategoryName: "Playeras", IsPrimary: true},
			},
		},
	}

	s.labels = []domain.StoreLabel{
		{
			ID: "lbl_jersey_norte", BranchID: "br_norte", StoreDisplayName: "LA MARINA",
			SKU: "CAAL686101YL1", MaterialCode: "CAAL686101YL1", Barcode: "7450130556398",
			SizeCode: "M", ColorCode: "YL1", Brand: "FEXPRO",
			PublicDescription: "CAMISETA MANGA CORTA DE HOMBRE", DepartmentLabel: "ROPA DEPORTIVA",
			ExtraDescriptions: map[string]any{"license": "Producto oficial Club América"},
			Currency: "MXN", CommonPrice: f64(899), SpecialPrice: f64(749), FinalPrice: f64(499),
			PriceMode: domain.PriceModeCommon, OnHand: f64(48),
		},
		{
			ID: "lbl_jersey_sur", BranchID: "br_sur", StoreDisplayName: "LA MARINA SUR",
			SKU: "CAAL686101YL1", MaterialCode: "CAAL686101YL1", Barcode: "7450130556398",
			SizeCode: "M", ColorCode: "YL1", Brand: "FEXPRO",
			PublicDescription: "CAMISETA MANGA CORTA DE HOMBRE", DepartmentLabel: "ROPA DEPORTIVA",
			Currency: "MXN", CommonPrice: f64(879), SpecialPrice: f64(699), FinalPrice: f64(449),
			PriceMode: domain.PriceModeSpecial, OnHand: f64(20),
		},
		{
			ID: "lbl_fig_norte", BranchID: "br_norte", StoreDisplayName: "NEXUS NORTE",
			SKU: "FIG-COL-01", MaterialCode: "FIG-COL-01", Barcode: "7500000000001",
			SizeCode: "U", ColorCode: "STD", Brand: "NEXUS",
			PublicDescription: "FIGURA COLECCIONABLE EDICIÓN LIMITADA", DepartmentLabel: "JUGUETERÍA",
			Currency: "MXN", CommonPrice: f64(599), SpecialPrice: f64(499), FinalPrice: f64(299),
			PriceMode: domain.PriceModeFinal, OnHand: f64(15),
		},
		{
			ID: "lbl_laptop_norte", BranchID: "br_norte", StoreDisplayName: "NEXUS NORTE",
			SKU: "LAPTOP-14", MaterialCode: "LAPTOP-14", Barcode: "7500000000014",
			SizeCode: "U", ColorCode: "STD", Brand: "NEXUS",
			PublicDescription: "LAPTOP 14 PULGADAS", DepartmentLabel: "ELECTRÓNICA",
			Currency: "MXN", CommonPrice: f64(12999), SpecialPrice: f64(11999),
			PriceMode: domain.PriceModeCommon, OnHand: f64(25),
		},
	}
	for i := range s.labels {
		s.labels[i].EffectivePrice = s.labels[i].ResolveEffectivePrice()
		s.labels[i].PriceLabel = priceLabel(s.labels[i].PriceMode)
	}
}

func (s *Memory) ListBalances(_ context.Context, filter domain.BalanceFilter) ([]domain.StockBalance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.StockBalance, 0, len(s.balances))
	for _, b := range s.balances {
		if filter.BranchCode != "" && b.BranchID != filter.BranchCode {
			continue
		}
		if filter.DepartmentCode != "" || filter.CategoryCode != "" {
			if !s.balanceMatchesPlacement(b, filter) {
				continue
			}
		}
		cp := *b
		out = append(out, cp)
	}
	return out, nil
}

func (s *Memory) balanceMatchesPlacement(b *domain.StockBalance, filter domain.BalanceFilter) bool {
	for _, p := range s.placements {
		if p.productSKU != b.SKUID {
			continue
		}
		if filter.BranchCode != "" && p.branchID != filter.BranchCode {
			continue
		}
		if filter.DepartmentCode != "" && p.deptCode != filter.DepartmentCode {
			continue
		}
		if filter.CategoryCode != "" && p.catCode != filter.CategoryCode {
			continue
		}
		return true
	}
	// Items without placements only show when no dept/category filter.
	return filter.DepartmentCode == "" && filter.CategoryCode == ""
}

func (s *Memory) ListDepartments(_ context.Context, _, branchCode string) ([]domain.Department, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Department, 0, len(s.departments))
	for _, d := range s.departments {
		if branchCode != "" && d.BranchID != branchCode {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Memory) ListCatalog(_ context.Context, filter domain.CatalogFilter) ([]domain.CatalogItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.CatalogItem, 0, len(s.catalog))
	for _, item := range s.catalog {
		placements := make([]domain.CatalogPlacement, 0, len(item.Placements))
		for _, p := range item.Placements {
			if filter.DepartmentCode != "" && p.DepartmentCode != filter.DepartmentCode {
				continue
			}
			if filter.CategoryCode != "" && p.CategoryCode != filter.CategoryCode {
				continue
			}
			placements = append(placements, p)
		}
		if filter.DepartmentCode != "" || filter.CategoryCode != "" {
			if len(placements) == 0 {
				continue
			}
		} else {
			placements = item.Placements
		}
		cp := item
		cp.Placements = placements
		out = append(out, cp)
	}
	return out, nil
}

func priceLabel(mode string) string {
	switch mode {
	case domain.PriceModeSpecial:
		return "Especial"
	case domain.PriceModeFinal:
		return "Final"
	default:
		return "Común"
	}
}

func (s *Memory) ListLabels(_ context.Context, filter domain.LabelFilter) ([]domain.StoreLabel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.StoreLabel, 0, len(s.labels))
	for _, l := range s.labels {
		if filter.BranchCode != "" && l.BranchID != filter.BranchCode {
			continue
		}
		if filter.SKUCode != "" && l.SKU != filter.SKUCode && l.MaterialCode != filter.SKUCode && l.Barcode != filter.SKUCode {
			continue
		}
		if filter.DepartmentCode != "" {
			match := false
			for _, p := range s.placements {
				if p.productSKU != l.SKU {
					continue
				}
				if filter.BranchCode != "" && p.branchID != filter.BranchCode {
					continue
				}
				if p.deptCode == filter.DepartmentCode {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		cp := l
		cp.EffectivePrice = cp.ResolveEffectivePrice()
		cp.PriceLabel = priceLabel(cp.PriceMode)
		out = append(out, cp)
	}
	return out, nil
}

func (s *Memory) PostMovement(_ context.Context, req domain.MovementRequest) (domain.Movement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.movements[req.IdempotencyKey]; ok {
		return existing, nil
	}
	if req.Quantity == 0 {
		return domain.Movement{}, errors.New("quantity required")
	}
	key := req.WarehouseID + "|" + req.SKUID
	bal, ok := s.balances[key]

	delta := req.Quantity
	switch strings.ToUpper(req.MovementType) {
	case "RECEIPT", "ADJUST_IN", "TRANSFER_IN":
		if delta < 0 {
			delta = -delta
		}
	case "ISSUE", "ADJUST_OUT", "TRANSFER_OUT":
		if delta > 0 {
			delta = -delta
		}
	case "ADJUST":
	default:
		return domain.Movement{}, errors.New("invalid movement_type")
	}

	if !ok {
		if delta <= 0 {
			return domain.Movement{}, domain.ErrNotFound
		}
		bal = &domain.StockBalance{
			ID: "bal_" + uuid.NewString(), WarehouseID: req.WarehouseID, BranchID: req.BranchID,
			SKUID: req.SKUID, SKU: req.SKUID, OnHand: 0, Version: 1,
		}
		s.balances[key] = bal
	}
	if req.ExpectedVersion != nil && bal.Version != *req.ExpectedVersion {
		return domain.Movement{}, domain.ErrConflict
	}

	next := bal.OnHand + delta
	if next < 0 {
		return domain.Movement{}, domain.ErrInsufficientStock
	}
	bal.OnHand = next
	bal.Version++

	mov := domain.Movement{
		ID:             "mov_" + uuid.NewString(),
		OrgID:          req.OrgID,
		BranchID:       req.BranchID,
		WarehouseID:    req.WarehouseID,
		SKUID:          req.SKUID,
		MovementType:   strings.ToUpper(req.MovementType),
		Quantity:       delta,
		Status:         "POSTED",
		PostedBy:       req.PostedBy,
		OperatorLabel:  req.OperatorLabel,
		SessionID:      req.SessionID,
		IdempotencyKey: req.IdempotencyKey,
		CreatedAt:      time.Now().UTC(),
	}
	s.movements[req.IdempotencyKey] = mov
	s.movementsByID[mov.ID] = mov
	return mov, nil
}

func (s *Memory) ListMovements(_ context.Context, filter domain.MovementFilter) ([]domain.Movement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := make([]domain.Movement, 0, len(s.movementsByID))
	for _, m := range s.movementsByID {
		if filter.BranchCode != "" && m.BranchID != filter.BranchCode {
			continue
		}
		if filter.WarehouseID != "" && m.WarehouseID != filter.WarehouseID {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Memory) GetMovement(_ context.Context, _, movementID string) (domain.Movement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.movementsByID[movementID]
	if !ok {
		return domain.Movement{}, domain.ErrNotFound
	}
	return m, nil
}

func (s *Memory) VoidMovement(_ context.Context, movementID string, req domain.VoidRequest) (domain.VoidResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.movements[req.IdempotencyKey]; ok {
		orig, okOrig := s.movementsByID[movementID]
		if !okOrig {
			return domain.VoidResult{}, domain.ErrNotFound
		}
		return domain.VoidResult{Original: orig, Compensation: existing}, nil
	}

	orig, ok := s.movementsByID[movementID]
	if !ok {
		return domain.VoidResult{}, domain.ErrNotFound
	}
	if orig.Status == "VOID" {
		return domain.VoidResult{}, domain.ErrAlreadyVoided
	}
	if orig.Status != "POSTED" {
		return domain.VoidResult{}, errors.New("only posted movements can be voided")
	}
	if req.Reason == "" {
		return domain.VoidResult{}, errors.New("reason required")
	}

	key := orig.WarehouseID + "|" + orig.SKUID
	bal, ok := s.balances[key]
	if !ok {
		return domain.VoidResult{}, domain.ErrNotFound
	}

	compDelta := -orig.Quantity
	next := bal.OnHand + compDelta
	if next < 0 {
		return domain.VoidResult{}, domain.ErrInsufficientStock
	}
	bal.OnHand = next
	bal.Version++

	now := time.Now().UTC()
	orig.Status = "VOID"
	orig.VoidReason = req.Reason
	orig.VoidedBy = req.VoidedBy
	orig.VoidedAt = &now
	s.movementsByID[orig.ID] = orig
	s.movements[orig.IdempotencyKey] = orig

	comp := domain.Movement{
		ID:             "mov_" + uuid.NewString(),
		OrgID:          orig.OrgID,
		BranchID:       orig.BranchID,
		WarehouseID:    orig.WarehouseID,
		SKUID:          orig.SKUID,
		MovementType:   "REVERSAL",
		Quantity:       compDelta,
		Status:         "POSTED",
		PostedBy:       req.VoidedBy,
		IdempotencyKey: req.IdempotencyKey,
		CreatedAt:      now,
		ReversalOf:     orig.ID,
	}
	s.movements[req.IdempotencyKey] = comp
	s.movementsByID[comp.ID] = comp

	return domain.VoidResult{Original: orig, Compensation: comp}, nil
}

func (s *Memory) ListWarehouses(_ context.Context, filter domain.WarehouseFilter) ([]domain.Warehouse, error) {
	out := []domain.Warehouse{
		{ID: "wh_norte", BranchID: "br_norte", Name: "Almacén Norte", Kind: domain.WarehouseKindStore},
		{ID: "wh_sur", BranchID: "br_sur", Name: "Almacén Sur", Kind: domain.WarehouseKindStore},
		{ID: "cedi_centro", BranchID: "br_cedi", Name: "CEDI Centro — recepción", Kind: domain.WarehouseKindCEDI},
		{ID: "principal", BranchID: "tienda", Name: "Almacén de llegada", Kind: domain.WarehouseKindArrival},
	}
	filtered := make([]domain.Warehouse, 0, len(out))
	for _, w := range out {
		if filter.BranchCode != "" && w.BranchID != filter.BranchCode {
			continue
		}
		if filter.Kind != "" && w.Kind != strings.ToUpper(filter.Kind) {
			continue
		}
		filtered = append(filtered, w)
	}
	return filtered, nil
}

func (s *Memory) CreateReceipt(_ context.Context, req domain.CreateReceiptRequest) (domain.Receipt, error) {
	return domain.Receipt{}, errors.New("receipts require postgres store")
}
func (s *Memory) ListReceipts(_ context.Context, _ domain.ReceiptFilter) ([]domain.Receipt, error) {
	return nil, nil
}
func (s *Memory) GetReceipt(_ context.Context, _, _ string) (domain.Receipt, error) {
	return domain.Receipt{}, domain.ErrNotFound
}
func (s *Memory) PostReceipt(_ context.Context, _, _, _ string) (domain.Receipt, error) {
	return domain.Receipt{}, domain.ErrNotFound
}

func (s *Memory) CreateShippingSlip(_ context.Context, _ domain.CreateShippingSlipRequest) (domain.ShippingSlip, error) {
	return domain.ShippingSlip{}, errors.New("shipping slips require postgres store")
}
func (s *Memory) ListShippingSlips(_ context.Context, _ domain.ShippingSlipFilter) ([]domain.ShippingSlip, error) {
	return nil, nil
}
func (s *Memory) GetShippingSlip(_ context.Context, _, _ string) (domain.ShippingSlip, error) {
	return domain.ShippingSlip{}, domain.ErrNotFound
}
func (s *Memory) MarkShippingSlipPrinted(_ context.Context, _, _, _ string) (domain.ShippingSlip, error) {
	return domain.ShippingSlip{}, domain.ErrNotFound
}
func (s *Memory) ShipShippingSlip(_ context.Context, _, _, _ string) (domain.ShippingSlip, error) {
	return domain.ShippingSlip{}, domain.ErrNotFound
}
func (s *Memory) ReceiveShippingSlip(_ context.Context, _, _, _ string) (domain.ShippingSlip, error) {
	return domain.ShippingSlip{}, domain.ErrNotFound
}
func (s *Memory) CancelShippingSlip(_ context.Context, _, _ string, _ domain.CancelParcelRequest) (domain.ShippingSlip, error) {
	return domain.ShippingSlip{}, domain.ErrNotFound
}

func (s *Memory) CreateTransportSheet(_ context.Context, _ domain.CreateTransportSheetRequest) (domain.TransportSheet, error) {
	return domain.TransportSheet{}, errors.New("transport sheets require postgres store")
}
func (s *Memory) ListTransportSheets(_ context.Context, _ domain.TransportSheetFilter) ([]domain.TransportSheet, error) {
	return nil, nil
}
func (s *Memory) GetTransportSheet(_ context.Context, _, _ string) (domain.TransportSheet, error) {
	return domain.TransportSheet{}, domain.ErrNotFound
}
func (s *Memory) MarkTransportSheetPrinted(_ context.Context, _, _, _ string) (domain.TransportSheet, error) {
	return domain.TransportSheet{}, domain.ErrNotFound
}
func (s *Memory) DepartTransportSheet(_ context.Context, _, _, _ string) (domain.TransportSheet, error) {
	return domain.TransportSheet{}, domain.ErrNotFound
}
func (s *Memory) DeliverTransportSheet(_ context.Context, _, _, _ string) (domain.TransportSheet, error) {
	return domain.TransportSheet{}, domain.ErrNotFound
}
func (s *Memory) CancelTransportSheet(_ context.Context, _, _ string, _ domain.CancelParcelRequest) (domain.TransportSheet, error) {
	return domain.TransportSheet{}, domain.ErrNotFound
}

func (s *Memory) CreateTransfer(_ context.Context, _ domain.CreateTransferRequest) (domain.InventoryTransfer, error) {
	return domain.InventoryTransfer{}, errors.New("transfers require postgres store")
}
func (s *Memory) ListTransfers(_ context.Context, _ domain.TransferFilter) ([]domain.InventoryTransfer, error) {
	return nil, nil
}
func (s *Memory) GetTransfer(_ context.Context, _, _ string) (domain.InventoryTransfer, error) {
	return domain.InventoryTransfer{}, domain.ErrNotFound
}
func (s *Memory) ShipTransfer(_ context.Context, _, _, _ string) (domain.InventoryTransfer, error) {
	return domain.InventoryTransfer{}, domain.ErrNotFound
}
func (s *Memory) ReceiveTransfer(_ context.Context, _, _, _ string) (domain.InventoryTransfer, error) {
	return domain.InventoryTransfer{}, domain.ErrNotFound
}
func (s *Memory) CancelTransfer(_ context.Context, _, _ string, _ domain.CancelTransferRequest) (domain.InventoryTransfer, error) {
	return domain.InventoryTransfer{}, domain.ErrNotFound
}

func (s *Memory) CreateWarrantyCase(_ context.Context, _ domain.CreateWarrantyCaseRequest) (domain.WarrantyCase, error) {
	return domain.WarrantyCase{}, errors.New("warranty cases require postgres store")
}
func (s *Memory) ListWarrantyCases(_ context.Context, _ domain.WarrantyCaseFilter) ([]domain.WarrantyCase, error) {
	return nil, nil
}
func (s *Memory) GetWarrantyCase(_ context.Context, _, _ string) (domain.WarrantyCase, error) {
	return domain.WarrantyCase{}, domain.ErrNotFound
}
func (s *Memory) CreateReturnCase(_ context.Context, _ domain.CreateReturnCaseRequest) (domain.ReturnCase, error) {
	return domain.ReturnCase{}, errors.New("return cases require postgres store")
}
func (s *Memory) ListReturnCases(_ context.Context, _ domain.ReturnCaseFilter) ([]domain.ReturnCase, error) {
	return nil, nil
}
func (s *Memory) GetReturnCase(_ context.Context, _, _ string) (domain.ReturnCase, error) {
	return domain.ReturnCase{}, domain.ErrNotFound
}
func (s *Memory) ListParcelHub(_ context.Context, _ domain.ParcelHubFilter) ([]domain.ParcelHubItem, error) {
	return nil, nil
}

func (s *Memory) GetStorefrontSettings(_ context.Context, _, _ string) (domain.StorefrontSettings, error) {
	return domain.StorefrontSettings{}, domain.ErrNotFound
}
func (s *Memory) UpsertStorefrontSettings(_ context.Context, _ domain.UpsertStorefrontRequest) (domain.StorefrontSettings, error) {
	return domain.StorefrontSettings{}, errors.New("storefront requires postgres store")
}
func (s *Memory) GetPublicStorefront(_ context.Context, _ string) (domain.StorefrontPublicView, error) {
	return domain.StorefrontPublicView{}, domain.ErrNotFound
}

func (s *Memory) RegisterCustomer(_ context.Context, _ domain.RegisterCustomerRequest) (domain.Customer, error) {
	return domain.Customer{}, errors.New("customers require postgres store")
}
func (s *Memory) AuthenticateCustomer(_ context.Context, _ domain.LoginCustomerRequest) (domain.Customer, error) {
	return domain.Customer{}, domain.ErrNotFound
}
func (s *Memory) GetCustomer(_ context.Context, _, _ string) (domain.Customer, error) {
	return domain.Customer{}, domain.ErrNotFound
}
func (s *Memory) ListCustomers(_ context.Context, _ domain.CustomerFilter) ([]domain.Customer, error) {
	return nil, nil
}
func (s *Memory) ListCustomerCards(_ context.Context, _, _ string) ([]domain.CustomerCard, error) {
	return nil, nil
}
func (s *Memory) CreateCustomerCard(_ context.Context, _ domain.CreateCustomerCardRequest) (domain.CustomerCard, error) {
	return domain.CustomerCard{}, errors.New("customer cards require postgres store")
}
func (s *Memory) BlockCustomerCard(_ context.Context, _, _ string, _ domain.BlockCardRequest) (domain.CustomerCard, error) {
	return domain.CustomerCard{}, domain.ErrNotFound
}
func (s *Memory) LookupCustomerCard(_ context.Context, _, _ string) (domain.CardLookupResult, error) {
	return domain.CardLookupResult{}, domain.ErrNotFound
}
func (s *Memory) ResolveOrgByStorefrontSlug(_ context.Context, _ string) (string, string, error) {
	return "", "", domain.ErrNotFound
}

func (s *Memory) GetFiscalSettings(_ context.Context, _, _ string) (domain.BranchFiscalSettings, error) {
	return domain.BranchFiscalSettings{
		PricesIncludeTax: true,
		DefaultTaxRate:   0.16,
		TicketSeries:     "T",
		InvoiceSeries:    "F",
		ReceiptFooter:    "Gracias por su compra.",
	}, nil
}
func (s *Memory) UpsertFiscalSettings(_ context.Context, _ domain.UpsertFiscalSettingsRequest) (domain.BranchFiscalSettings, error) {
	return domain.BranchFiscalSettings{}, errors.New("pos fiscal settings require postgres store")
}
func (s *Memory) CompleteSale(_ context.Context, _ domain.CompleteSaleRequest) (domain.POSSale, error) {
	return domain.POSSale{}, errors.New("pos sales require postgres store")
}
func (s *Memory) GetSale(_ context.Context, _, _ string) (domain.POSSale, error) {
	return domain.POSSale{}, domain.ErrNotFound
}
func (s *Memory) ListSales(_ context.Context, _ domain.SaleFilter) ([]domain.POSSale, error) {
	return nil, nil
}
func (s *Memory) RequestSaleInvoice(_ context.Context, _, _ string, _ domain.RequestInvoiceRequest) (domain.FiscalInvoice, error) {
	return domain.FiscalInvoice{}, errors.New("pos invoices require postgres store")
}

func (s *Memory) CreateInboundShipment(_ context.Context, _ domain.CreateInboundShipmentRequest) (domain.InboundShipment, error) {
	return domain.InboundShipment{}, errors.New("inbound shipments require postgres store")
}
func (s *Memory) GetInboundShipment(_ context.Context, _, _ string) (domain.InboundShipment, error) {
	return domain.InboundShipment{}, domain.ErrNotFound
}
func (s *Memory) ListInboundShipments(_ context.Context, _ domain.InboundShipmentFilter) ([]domain.InboundShipment, error) {
	return nil, nil
}
func (s *Memory) PostInboundShipment(_ context.Context, _, _, _ string) (domain.InboundShipment, error) {
	return domain.InboundShipment{}, errors.New("inbound shipments require postgres store")
}
