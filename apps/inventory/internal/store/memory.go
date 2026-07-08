package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
)

type Memory struct {
	mu        sync.Mutex
	balances  map[string]*domain.StockBalance
	movements map[string]domain.Movement
}

func NewMemory() *Memory {
	return &Memory{
		balances:  map[string]*domain.StockBalance{},
		movements: map[string]domain.Movement{},
	}
}

func (s *Memory) SeedDemo() {
	items := []domain.StockBalance{
		{ID: "bal_1", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "BOLT-M8", SKU: "BOLT-M8", OnHand: 1000, Version: 1},
		{ID: "bal_2", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "NUT-M8", SKU: "NUT-M8", OnHand: 800, Version: 1},
		{ID: "bal_3", WarehouseID: "wh_sur", BranchID: "br_sur", SKUID: "BOLT-M8", SKU: "BOLT-M8", OnHand: 400, Version: 1},
	}
	for i := range items {
		b := items[i]
		key := b.WarehouseID + "|" + b.SKUID
		cp := b
		s.balances[key] = &cp
	}
}

func (s *Memory) ListBalances(_ context.Context, _, branchID string) ([]domain.StockBalance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.StockBalance, 0, len(s.balances))
	for _, b := range s.balances {
		if branchID != "" && b.BranchID != branchID {
			continue
		}
		out = append(out, *b)
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
	if !ok {
		return domain.Movement{}, domain.ErrNotFound
	}
	if req.ExpectedVersion != nil && bal.Version != *req.ExpectedVersion {
		return domain.Movement{}, domain.ErrConflict
	}

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
		IdempotencyKey: req.IdempotencyKey,
		CreatedAt:      time.Now().UTC(),
	}
	s.movements[req.IdempotencyKey] = mov
	return mov, nil
}
