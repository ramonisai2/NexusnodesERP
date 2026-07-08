package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

func main() {
	addr := envOr("INVENTORY_ADDR", ":8082")
	store := NewMemoryStore()
	_ = store.SeedDemo()

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(20 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "inventory"})
	})

	r.Get("/balances", func(w http.ResponseWriter, req *http.Request) {
		if !hasPerm(req, "inventory.balance.read") {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		branchID := req.Header.Get("X-Branch-Id")
		writeJSON(w, http.StatusOK, store.ListBalances(branchID))
	})

	r.Post("/movements", func(w http.ResponseWriter, req *http.Request) {
		if !hasPerm(req, "inventory.movement.create") {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		var body MovementRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		idem := req.Header.Get("Idempotency-Key")
		if idem == "" {
			idem = body.IdempotencyKey
		}
		if idem == "" {
			http.Error(w, `{"error":"idempotency_key_required"}`, http.StatusBadRequest)
			return
		}
		body.IdempotencyKey = idem
		body.PostedBy = req.Header.Get("X-User-Id")
		body.OrgID = req.Header.Get("X-Org-Id")

		if !branchAllowed(req, body.BranchID) {
			http.Error(w, `{"error":"branch_forbidden"}`, http.StatusForbidden)
			return
		}

		mov, err := store.PostMovement(req.Context(), body)
		if errors.Is(err, ErrConflict) {
			http.Error(w, `{"error":"version_conflict"}`, http.StatusConflict)
			return
		}
		if errors.Is(err, ErrInsufficientStock) {
			http.Error(w, `{"error":"insufficient_stock"}`, http.StatusUnprocessableEntity)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"post_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, mov)
	})

	log.Printf("inventory listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

func hasPerm(r *http.Request, code string) bool {
	perms := strings.Split(r.Header.Get("X-Permissions"), ",")
	for _, p := range perms {
		if strings.TrimSpace(p) == code {
			return true
		}
	}
	// Local direct calls without gateway: allow in DEV
	if strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true") && r.Header.Get("X-Permissions") == "" {
		return true
	}
	return false
}

func branchAllowed(r *http.Request, branchID string) bool {
	raw := r.Header.Get("X-Branch-Ids")
	if raw == "" && strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true") {
		return true
	}
	for _, b := range strings.Split(raw, ",") {
		b = strings.TrimSpace(b)
		if b == branchID || b == "*" {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// --- Domain (in-memory vertical slice; Postgres store swaps in later) ---

var (
	ErrConflict          = errors.New("version conflict")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrNotFound          = errors.New("not found")
)

type StockBalance struct {
	ID          string  `json:"id"`
	WarehouseID string  `json:"warehouse_id"`
	BranchID    string  `json:"branch_id"`
	SKUID       string  `json:"sku_id"`
	SKU         string  `json:"sku"`
	OnHand      float64 `json:"on_hand"`
	Reserved    float64 `json:"reserved"`
	Version     int     `json:"version"`
}

type MovementRequest struct {
	OrgID          string  `json:"org_id"`
	BranchID       string  `json:"branch_id"`
	WarehouseID    string  `json:"warehouse_id"`
	SKUID          string  `json:"sku_id"`
	MovementType   string  `json:"movement_type"`
	Quantity       float64 `json:"quantity"`
	ExpectedVersion *int   `json:"expected_version"`
	IdempotencyKey string  `json:"idempotency_key"`
	PostedBy       string  `json:"posted_by"`
}

type Movement struct {
	ID             string    `json:"id"`
	OrgID          string    `json:"org_id"`
	BranchID       string    `json:"branch_id"`
	WarehouseID    string    `json:"warehouse_id"`
	SKUID          string    `json:"sku_id"`
	MovementType   string    `json:"movement_type"`
	Quantity       float64   `json:"quantity"`
	Status         string    `json:"status"`
	PostedBy       string    `json:"posted_by"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

type MemoryStore struct {
	mu         sync.Mutex
	balances   map[string]*StockBalance // key warehouse|sku
	movements  map[string]Movement      // idempotency -> movement
	byID       map[string]*StockBalance
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		balances:  map[string]*StockBalance{},
		movements: map[string]Movement{},
		byID:      map[string]*StockBalance{},
	}
}

func (s *MemoryStore) SeedDemo() error {
	items := []StockBalance{
		{ID: "bal_1", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "sku_bolt", SKU: "BOLT-M8", OnHand: 1000, Version: 1},
		{ID: "bal_2", WarehouseID: "wh_norte", BranchID: "br_norte", SKUID: "sku_nut", SKU: "NUT-M8", OnHand: 800, Version: 1},
		{ID: "bal_3", WarehouseID: "wh_sur", BranchID: "br_sur", SKUID: "sku_bolt", SKU: "BOLT-M8", OnHand: 400, Version: 1},
	}
	for i := range items {
		b := items[i]
		key := b.WarehouseID + "|" + b.SKUID
		cp := b
		s.balances[key] = &cp
		s.byID[b.ID] = &cp
	}
	return nil
}

func (s *MemoryStore) ListBalances(branchID string) []StockBalance {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StockBalance, 0, len(s.balances))
	for _, b := range s.balances {
		if branchID != "" && b.BranchID != branchID {
			continue
		}
		out = append(out, *b)
	}
	return out
}

func (s *MemoryStore) PostMovement(_ context.Context, req MovementRequest) (Movement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.movements[req.IdempotencyKey]; ok {
		return existing, nil
	}
	if req.Quantity == 0 {
		return Movement{}, errors.New("quantity required")
	}
	key := req.WarehouseID + "|" + req.SKUID
	bal, ok := s.balances[key]
	if !ok {
		return Movement{}, ErrNotFound
	}
	if req.ExpectedVersion != nil && bal.Version != *req.ExpectedVersion {
		return Movement{}, ErrConflict
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
		// signed quantity as-is
	default:
		return Movement{}, errors.New("invalid movement_type")
	}

	next := bal.OnHand + delta
	if next < 0 {
		return Movement{}, ErrInsufficientStock
	}
	bal.OnHand = next
	bal.Version++

	mov := Movement{
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
