package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/store"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
)

func main() {
	addr := envOr("INVENTORY_ADDR", ":8082")
	ctx := context.Background()
	shutdown, err := otelx.Init(ctx, "inventory")
	if err != nil {
		log.Fatalf("otel: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	var inventoryStore domain.Store
	if os.Getenv("DATABASE_URL") != "" {
		pool, err := db.Connect(ctx)
		if err != nil {
			log.Fatalf("postgres: %v", err)
		}
		inventoryStore = store.NewPostgres(pool)
		log.Printf("inventory store=postgres")
	} else {
		mem := store.NewMemory()
		mem.SeedDemo()
		inventoryStore = mem
		log.Printf("inventory store=memory")
	}

	opa := authz.NewClientFromEnv()

	r := chi.NewRouter()
	r.Use(otelx.Middleware("inventory"))
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(20 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "inventory"})
	})

	r.Get("/balances", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.Header.Get("X-Branch-Id")
		if q := req.URL.Query().Get("branch_id"); q != "" {
			branchID = q
		}
		dept := req.URL.Query().Get("department")
		cat := req.URL.Query().Get("category")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.balance.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.balance.read")
			return
		}
		balances, err := inventoryStore.ListBalances(req.Context(), domain.BalanceFilter{
			OrgRef:         subject.OrgID,
			BranchCode:     branchID,
			DepartmentCode: dept,
			CategoryCode:   cat,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, balances)
	})

	r.Get("/departments", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.Header.Get("X-Branch-Id")
		if q := req.URL.Query().Get("branch_id"); q != "" {
			branchID = q
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.catalog.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			// Fallback: anyone who can read balances can browse departments.
			allowBal, err2 := opa.Allow(req.Context(), authz.Input{
				Subject:  subject,
				Action:   "inventory.balance.read",
				Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
			})
			if err2 != nil || !allowBal {
				authz.WriteForbidden(w, "inventory.catalog.read")
				return
			}
		}
		deps, err := inventoryStore.ListDepartments(req.Context(), subject.OrgID, branchID)
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, deps)
	})

	r.Get("/catalog", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.Header.Get("X-Branch-Id")
		if q := req.URL.Query().Get("branch_id"); q != "" {
			branchID = q
		}
		dept := req.URL.Query().Get("department")
		cat := req.URL.Query().Get("category")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.catalog.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			allowBal, err2 := opa.Allow(req.Context(), authz.Input{
				Subject:  subject,
				Action:   "inventory.balance.read",
				Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
			})
			if err2 != nil || !allowBal {
				authz.WriteForbidden(w, "inventory.catalog.read")
				return
			}
		}
		items, err := inventoryStore.ListCatalog(req.Context(), domain.CatalogFilter{
			OrgRef:         subject.OrgID,
			BranchCode:     branchID,
			DepartmentCode: dept,
			CategoryCode:   cat,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/labels", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.Header.Get("X-Branch-Id")
		if q := req.URL.Query().Get("branch_id"); q != "" {
			branchID = q
		}
		sku := req.URL.Query().Get("sku")
		dept := req.URL.Query().Get("department")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.label.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			allowBal, err2 := opa.Allow(req.Context(), authz.Input{
				Subject:  subject,
				Action:   "inventory.balance.read",
				Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
			})
			if err2 != nil || !allowBal {
				authz.WriteForbidden(w, "inventory.label.read")
				return
			}
		}
		labels, err := inventoryStore.ListLabels(req.Context(), domain.LabelFilter{
			OrgRef:         subject.OrgID,
			BranchCode:     branchID,
			SKUCode:        sku,
			DepartmentCode: dept,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, labels)
	})

	r.Get("/movements", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.Header.Get("X-Branch-Id")
		if q := req.URL.Query().Get("branch_id"); q != "" {
			branchID = q
		}
		warehouseID := req.URL.Query().Get("warehouse_id")
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))

		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.movement.read",
			Resource: map[string]any{
				"branch_id":    branchID,
				"warehouse_id": warehouseID,
				"org_id":       subject.OrgID,
			},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.movement.read")
			return
		}

		movements, err := inventoryStore.ListMovements(req.Context(), domain.MovementFilter{
			OrgRef:      subject.OrgID,
			BranchCode:  branchID,
			WarehouseID: warehouseID,
			Limit:       limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, movements)
	})

	r.Post("/movements", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.MovementRequest
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
		if body.PostedBy == "" {
			body.PostedBy = subject.Sub
		}
		if body.OrgID == "" {
			body.OrgID = subject.OrgID
		}
		if body.OperatorLabel == "" {
			body.OperatorLabel = subject.OperatorLabel
		}
		if body.SessionID == "" {
			body.SessionID = subject.SessionID
		}

		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.movement.create",
			Resource: map[string]any{
				"branch_id":    body.BranchID,
				"warehouse_id": body.WarehouseID,
				"org_id":       body.OrgID,
				"quantity":     body.Quantity,
			},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.movement.create")
			return
		}

		mov, err := inventoryStore.PostMovement(req.Context(), body)
		if errors.Is(err, domain.ErrConflict) {
			http.Error(w, `{"error":"version_conflict"}`, http.StatusConflict)
			return
		}
		if errors.Is(err, domain.ErrInsufficientStock) {
			http.Error(w, `{"error":"insufficient_stock"}`, http.StatusUnprocessableEntity)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"post_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, mov)
	})

	r.Post("/movements/{id}/void", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		movementID := chi.URLParam(req, "id")
		if movementID == "" {
			http.Error(w, `{"error":"movement_id_required"}`, http.StatusBadRequest)
			return
		}

		var body domain.VoidRequest
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
		if body.VoidedBy == "" {
			body.VoidedBy = subject.Sub
		}
		if body.OrgID == "" {
			body.OrgID = subject.OrgID
		}
		if body.OperatorLabel == "" {
			body.OperatorLabel = subject.OperatorLabel
		}
		if body.SessionID == "" {
			body.SessionID = subject.SessionID
		}

		mov, err := inventoryStore.GetMovement(req.Context(), subject.OrgID, movementID)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		resource := map[string]any{
			"branch_id":    mov.BranchID,
			"warehouse_id": mov.WarehouseID,
			"org_id":       subject.OrgID,
			"movement_id":  mov.ID,
		}

		// Direct void for managers; clerks get 403 and should use /approvals (maker-checker).
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.movement.void",
			Resource: resource,
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			// Hint: clerks with void.request should open an approval instead.
			if subject.HasPermission("inventory.movement.void.request") {
				http.Error(w, `{"error":"approval_required","hint":"POST /approvals with action inventory.movement.void"}`, http.StatusForbidden)
				return
			}
			authz.WriteForbidden(w, "inventory.movement.void")
			return
		}

		result, err := inventoryStore.VoidMovement(req.Context(), movementID, body)
		if errors.Is(err, domain.ErrAlreadyVoided) {
			http.Error(w, `{"error":"already_voided"}`, http.StatusConflict)
			return
		}
		if errors.Is(err, domain.ErrInsufficientStock) {
			http.Error(w, `{"error":"insufficient_stock"}`, http.StatusUnprocessableEntity)
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			http.Error(w, `{"error":"version_conflict"}`, http.StatusConflict)
			return
		}
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"void_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	r.Get("/warehouses", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		kind := req.URL.Query().Get("kind")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.warehouse.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.warehouse.read")
			return
		}
		items, err := inventoryStore.ListWarehouses(req.Context(), domain.WarehouseFilter{
			OrgRef: subject.OrgID, BranchCode: branchID, Kind: kind,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/receipts", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.receipt.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.receipt.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		items, err := inventoryStore.ListReceipts(req.Context(), domain.ReceiptFilter{
			OrgRef: subject.OrgID, BranchCode: branchID,
			WarehouseID: req.URL.Query().Get("warehouse_id"),
			Status:      req.URL.Query().Get("status"),
			Limit:       limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/receipts/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.receipt.read",
			Resource: map[string]any{"org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.receipt.read")
			return
		}
		rec, err := inventoryStore.GetReceipt(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, rec)
	})

	r.Post("/receipts", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CreateReceiptRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		idem := req.Header.Get("Idempotency-Key")
		if idem == "" {
			idem = body.IdempotencyKey
		}
		body.IdempotencyKey = idem
		if body.OrgID == "" {
			body.OrgID = subject.OrgID
		}
		if body.CreatedBy == "" {
			body.CreatedBy = subject.Sub
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.receipt.create",
			Resource: map[string]any{
				"branch_id":    body.BranchID,
				"warehouse_id": body.WarehouseID,
				"org_id":       body.OrgID,
			},
			Context: map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.receipt.create")
			return
		}
		rec, err := inventoryStore.CreateReceipt(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, rec)
	})

	r.Post("/receipts/{id}/post", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.receipt.post",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.receipt.post")
			return
		}
		rec, err := inventoryStore.PostReceipt(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			http.Error(w, `{"error":"version_conflict"}`, http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"post_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, rec)
	})

	r.Get("/slips", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.slip.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.slip.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		direction := req.URL.Query().Get("direction") // from|to|both
		filter := domain.ShippingSlipFilter{
			OrgRef: subject.OrgID,
			Status: req.URL.Query().Get("status"),
			Limit:  limit,
		}
		switch direction {
		case "to":
			filter.ToBranch = branchID
		case "from", "":
			filter.FromBranch = branchID
		}
		items, err := inventoryStore.ListShippingSlips(req.Context(), filter)
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/slips/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.slip.read",
			Resource: map[string]any{"org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.slip.read")
			return
		}
		slip, err := inventoryStore.GetShippingSlip(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, slip)
	})

	r.Post("/slips", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CreateShippingSlipRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		idem := req.Header.Get("Idempotency-Key")
		if idem == "" {
			idem = body.IdempotencyKey
		}
		body.IdempotencyKey = idem
		if body.OrgID == "" {
			body.OrgID = subject.OrgID
		}
		if body.CreatedBy == "" {
			body.CreatedBy = subject.Sub
		}
		if body.OperatorLabel == "" {
			body.OperatorLabel = subject.OperatorLabel
		}
		if body.SessionID == "" {
			body.SessionID = subject.SessionID
		}
		if body.FromBranchID == "" {
			body.FromBranchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.slip.create",
			Resource: map[string]any{
				"branch_id": body.FromBranchID,
				"org_id":    body.OrgID,
			},
			Context: map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.slip.create")
			return
		}
		slip, err := inventoryStore.CreateShippingSlip(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, slip)
	})

	r.Post("/slips/{id}/print", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.slip.print",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.slip.print")
			return
		}
		slip, err := inventoryStore.MarkShippingSlipPrinted(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"print_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, slip)
	})

	r.Get("/transport-sheets", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.transport.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transport.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		filter := domain.TransportSheetFilter{
			OrgRef: subject.OrgID,
			Status: req.URL.Query().Get("status"),
			Limit:  limit,
		}
		if req.URL.Query().Get("direction") == "to" {
			filter.ToBranch = branchID
		} else {
			filter.FromBranch = branchID
		}
		items, err := inventoryStore.ListTransportSheets(req.Context(), filter)
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/transport-sheets/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.transport.read",
			Resource: map[string]any{"org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transport.read")
			return
		}
		sheet, err := inventoryStore.GetTransportSheet(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, sheet)
	})

	r.Post("/transport-sheets", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CreateTransportSheetRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		idem := req.Header.Get("Idempotency-Key")
		if idem == "" {
			idem = body.IdempotencyKey
		}
		body.IdempotencyKey = idem
		if body.OrgID == "" {
			body.OrgID = subject.OrgID
		}
		if body.CreatedBy == "" {
			body.CreatedBy = subject.Sub
		}
		if body.OperatorLabel == "" {
			body.OperatorLabel = subject.OperatorLabel
		}
		if body.SessionID == "" {
			body.SessionID = subject.SessionID
		}
		if body.FromBranchID == "" {
			body.FromBranchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.transport.create",
			Resource: map[string]any{
				"branch_id": body.FromBranchID,
				"org_id":    body.OrgID,
			},
			Context: map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transport.create")
			return
		}
		sheet, err := inventoryStore.CreateTransportSheet(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, sheet)
	})

	r.Post("/transport-sheets/{id}/print", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.transport.print",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transport.print")
			return
		}
		sheet, err := inventoryStore.MarkTransportSheetPrinted(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"print_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, sheet)
	})

	r.Get("/transfers", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.transfer.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transfer.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		filter := domain.TransferFilter{
			OrgRef: subject.OrgID,
			Status: req.URL.Query().Get("status"),
			Limit:  limit,
		}
		switch req.URL.Query().Get("direction") {
		case "to":
			filter.ToBranch = branchID
		case "all":
			// no branch filter
		default:
			filter.FromBranch = branchID
		}
		items, err := inventoryStore.ListTransfers(req.Context(), filter)
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.InventoryTransfer{}
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/transfers/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.transfer.read",
			Resource: map[string]any{"org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transfer.read")
			return
		}
		tr, err := inventoryStore.GetTransfer(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, tr)
	})

	r.Post("/transfers", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CreateTransferRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.CreatedBy = subject.Sub
		if body.OperatorLabel == "" {
			body.OperatorLabel = subject.OperatorLabel
		}
		if body.SessionID == "" {
			body.SessionID = subject.SessionID
		}
		if body.FromBranchID == "" {
			body.FromBranchID = req.Header.Get("X-Branch-Id")
		}
		if body.IdempotencyKey == "" {
			body.IdempotencyKey = req.Header.Get("Idempotency-Key")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.transfer.create",
			Resource: map[string]any{
				"branch_id": body.FromBranchID,
				"org_id":    body.OrgID,
			},
			Context: map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transfer.create")
			return
		}
		tr, err := inventoryStore.CreateTransfer(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, tr)
	})

	r.Post("/transfers/{id}/ship", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		tr, err := inventoryStore.GetTransfer(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
			return
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.transfer.ship",
			Resource: map[string]any{
				"branch_id": tr.FromBranchID,
				"org_id":    subject.OrgID,
			},
			Context: map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transfer.ship")
			return
		}
		out, err := inventoryStore.ShipTransfer(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrInsufficientStock) {
			http.Error(w, `{"error":"insufficient_stock"}`, http.StatusConflict)
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			http.Error(w, `{"error":"version_conflict"}`, http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"ship_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	r.Post("/transfers/{id}/receive", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		tr, err := inventoryStore.GetTransfer(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
			return
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.transfer.receive",
			Resource: map[string]any{
				"branch_id": tr.ToBranchID,
				"org_id":    subject.OrgID,
			},
			Context: map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transfer.receive")
			return
		}
		out, err := inventoryStore.ReceiveTransfer(req.Context(), subject.OrgID, id, subject.Sub)
		if err != nil {
			http.Error(w, `{"error":"receive_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	r.Post("/transfers/{id}/cancel", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		var body domain.CancelTransferRequest
		_ = json.NewDecoder(req.Body).Decode(&body)
		body.Actor = subject.Sub
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.transfer.cancel",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transfer.cancel")
			return
		}
		out, err := inventoryStore.CancelTransfer(req.Context(), subject.OrgID, id, body)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"cancel_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	log.Printf("inventory listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
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

func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return s
}
