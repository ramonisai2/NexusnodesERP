package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/store"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
	"github.com/ramonisai2/NexusnodesERP/packages/go/secure"
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
	r.Use(secure.SecurityHeadersMiddleware)
	r.Use(secure.LimitBodyMiddleware(1 << 20))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "inventory"})
	})

	// Public storefront (no JWT) — gateway exposes /storefront/{slug} without auth.
	r.Get("/storefront/public/{slug}", func(w http.ResponseWriter, req *http.Request) {
		slug := chi.URLParam(req, "slug")
		view, err := inventoryStore.GetPublicStorefront(req.Context(), slug)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, view)
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
		reasonCode := req.URL.Query().Get("reason_code")
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
			ReasonCode:  reasonCode,
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
			OrgRef:     subject.OrgID,
			Status:     req.URL.Query().Get("status"),
			ParcelKind: req.URL.Query().Get("parcel_kind"),
			Limit:      limit,
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

	r.Post("/slips/{id}/ship", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.slip.ship",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.slip.ship")
			return
		}
		slip, err := inventoryStore.ShipShippingSlip(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"ship_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, slip)
	})

	r.Post("/slips/{id}/receive", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.slip.receive",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.slip.receive")
			return
		}
		slip, err := inventoryStore.ReceiveShippingSlip(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"receive_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, slip)
	})

	r.Post("/slips/{id}/cancel", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		var body domain.CancelParcelRequest
		_ = json.NewDecoder(req.Body).Decode(&body)
		body.Actor = subject.Sub
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.slip.cancel",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.slip.cancel")
			return
		}
		slip, err := inventoryStore.CancelShippingSlip(req.Context(), subject.OrgID, id, body)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"cancel_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
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
			OrgRef:     subject.OrgID,
			Status:     req.URL.Query().Get("status"),
			ParcelKind: req.URL.Query().Get("parcel_kind"),
			Limit:      limit,
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

	r.Post("/transport-sheets/{id}/depart", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.transport.depart",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transport.depart")
			return
		}
		sheet, err := inventoryStore.DepartTransportSheet(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"depart_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, sheet)
	})

	r.Post("/transport-sheets/{id}/deliver", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.transport.deliver",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transport.deliver")
			return
		}
		sheet, err := inventoryStore.DeliverTransportSheet(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"deliver_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, sheet)
	})

	r.Post("/transport-sheets/{id}/cancel", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		var body domain.CancelParcelRequest
		_ = json.NewDecoder(req.Body).Decode(&body)
		body.Actor = subject.Sub
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.transport.cancel",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.transport.cancel")
			return
		}
		sheet, err := inventoryStore.CancelTransportSheet(req.Context(), subject.OrgID, id, body)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"cancel_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
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
			OrgRef:     subject.OrgID,
			Status:     req.URL.Query().Get("status"),
			ParcelKind: req.URL.Query().Get("parcel_kind"),
			Limit:      limit,
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

	r.Get("/parcels", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.parcel.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.parcel.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		items, err := inventoryStore.ListParcelHub(req.Context(), domain.ParcelHubFilter{
			OrgRef:     subject.OrgID,
			BranchID:   branchID,
			ParcelKind: req.URL.Query().Get("parcel_kind"),
			Limit:      limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.ParcelHubItem{}
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/warranty-cases", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.warranty.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.warranty.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		items, err := inventoryStore.ListWarrantyCases(req.Context(), domain.WarrantyCaseFilter{
			OrgRef:   subject.OrgID,
			BranchID: branchID,
			Status:   req.URL.Query().Get("status"),
			Limit:    limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.WarrantyCase{}
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/warranty-cases/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.warranty.read",
			Resource: map[string]any{"org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.warranty.read")
			return
		}
		item, err := inventoryStore.GetWarrantyCase(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	r.Post("/warranty-cases", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CreateWarrantyCaseRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.CreatedBy = subject.Sub
		if body.OperatorLabel == "" {
			body.OperatorLabel = subject.OperatorLabel
		}
		if body.BranchID == "" {
			body.BranchID = req.Header.Get("X-Branch-Id")
		}
		if body.IdempotencyKey == "" {
			body.IdempotencyKey = req.Header.Get("Idempotency-Key")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.warranty.create",
			Resource: map[string]any{
				"branch_id": body.BranchID,
				"org_id":    body.OrgID,
			},
			Context: map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.warranty.create")
			return
		}
		item, err := inventoryStore.CreateWarrantyCase(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})

	r.Get("/return-cases", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.return.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.return.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		items, err := inventoryStore.ListReturnCases(req.Context(), domain.ReturnCaseFilter{
			OrgRef:     subject.OrgID,
			FromBranch: branchID,
			Status:     req.URL.Query().Get("status"),
			Limit:      limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.ReturnCase{}
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/return-cases/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.return.read",
			Resource: map[string]any{"org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.return.read")
			return
		}
		item, err := inventoryStore.GetReturnCase(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	r.Post("/return-cases", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CreateReturnCaseRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.CreatedBy = subject.Sub
		if body.OperatorLabel == "" {
			body.OperatorLabel = subject.OperatorLabel
		}
		if body.FromBranchID == "" {
			body.FromBranchID = req.Header.Get("X-Branch-Id")
		}
		if body.IdempotencyKey == "" {
			body.IdempotencyKey = req.Header.Get("Idempotency-Key")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.return.create",
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
			authz.WriteForbidden(w, "inventory.return.create")
			return
		}
		item, err := inventoryStore.CreateReturnCase(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})

	r.Get("/storefront/settings", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "store.storefront.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "store.storefront.read")
			return
		}
		settings, err := inventoryStore.GetStorefrontSettings(req.Context(), subject.OrgID, branchID)
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	r.Put("/storefront/settings", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.UpsertStorefrontRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.UpdatedBy = subject.Sub
		if body.BranchID == "" {
			body.BranchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "store.storefront.manage",
			Resource: map[string]any{
				"branch_id": body.BranchID,
				"org_id":    subject.OrgID,
			},
			Context: map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "store.storefront.manage")
			return
		}
		settings, err := inventoryStore.UpsertStorefrontSettings(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"save_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	// —— Public customer registration / login (no JWT; gateway mints customer token) ——
	r.Post("/customers/register", func(w http.ResponseWriter, req *http.Request) {
		var body domain.RegisterCustomerRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		cust, err := inventoryStore.RegisterCustomer(req.Context(), body)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, `{"error":"register_failed","detail":"`+escapeJSON(err.Error())+`"}`, status)
			return
		}
		writeJSON(w, http.StatusCreated, cust)
	})

	r.Post("/customers/login", func(w http.ResponseWriter, req *http.Request) {
		var body domain.LoginCustomerRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		cust, err := inventoryStore.AuthenticateCustomer(req.Context(), body)
		if errors.Is(err, domain.ErrNotFound) || (err != nil && err.Error() == "invalid credentials") {
			http.Error(w, `{"error":"invalid_credentials"}`, http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"login_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, cust)
	})

	r.Get("/customers/me", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		customerID := customerIDFromSubject(subject)
		if customerID == "" {
			authz.WriteForbidden(w, "customer.self.read")
			return
		}
		if !subject.HasPermission("customer.self.read") && !subject.HasPermission("customer.read") {
			authz.WriteForbidden(w, "customer.self.read")
			return
		}
		cust, err := inventoryStore.GetCustomer(req.Context(), subject.OrgID, customerID)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, cust)
	})

	r.Get("/customers/me/cards", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		customerID := customerIDFromSubject(subject)
		if customerID == "" || (!subject.HasPermission("customer.card.self") && !subject.HasPermission("customer.card.read")) {
			authz.WriteForbidden(w, "customer.card.self")
			return
		}
		cards, err := inventoryStore.ListCustomerCards(req.Context(), subject.OrgID, customerID)
		if err != nil {
			http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
			return
		}
		if cards == nil {
			cards = []domain.CustomerCard{}
		}
		writeJSON(w, http.StatusOK, cards)
	})

	r.Post("/customers/me/cards", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		customerID := customerIDFromSubject(subject)
		if customerID == "" || (!subject.HasPermission("customer.card.self") && !subject.HasPermission("customer.card.manage")) {
			authz.WriteForbidden(w, "customer.card.self")
			return
		}
		var body domain.CreateCustomerCardRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.CustomerID = customerID
		body.IssuedBy = subject.Sub
		card, err := inventoryStore.CreateCustomerCard(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, card)
	})

	r.Post("/customers/me/cards/{id}/block", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		customerID := customerIDFromSubject(subject)
		if customerID == "" || (!subject.HasPermission("customer.card.self") && !subject.HasPermission("customer.card.manage")) {
			authz.WriteForbidden(w, "customer.card.self")
			return
		}
		id := chi.URLParam(req, "id")
		var body domain.BlockCardRequest
		_ = json.NewDecoder(req.Body).Decode(&body)
		body.Actor = subject.Sub
		// Ensure card belongs to this customer.
		cards, err := inventoryStore.ListCustomerCards(req.Context(), subject.OrgID, customerID)
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
			return
		}
		owned := false
		for _, c := range cards {
			if c.ID == id {
				owned = true
				break
			}
		}
		if !owned {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		card, err := inventoryStore.BlockCustomerCard(req.Context(), subject.OrgID, id, body)
		if err != nil {
			http.Error(w, `{"error":"block_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, card)
	})

	// —— Staff customer directory ——
	r.Get("/customers", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "customer.read",
			Resource: map[string]any{"org_id": subject.OrgID, "branch_id": req.Header.Get("X-Branch-Id")},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "customer.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		items, err := inventoryStore.ListCustomers(req.Context(), domain.CustomerFilter{
			OrgRef: subject.OrgID,
			Query:  req.URL.Query().Get("q"),
			Status: req.URL.Query().Get("status"),
			Limit:  limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.Customer{}
		}
		writeJSON(w, http.StatusOK, items)
	})

	r.Get("/customers/cards/lookup", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		code := req.URL.Query().Get("code")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "customer.card.read",
			Resource: map[string]any{"org_id": subject.OrgID, "branch_id": req.Header.Get("X-Branch-Id")},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "customer.card.read")
			return
		}
		result, err := inventoryStore.LookupCustomerCard(req.Context(), subject.OrgID, code)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	r.Get("/customers/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "customer.read",
			Resource: map[string]any{"org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "customer.read")
			return
		}
		cust, err := inventoryStore.GetCustomer(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, cust)
	})

	r.Post("/customers/{id}/cards", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "customer.card.manage",
			Resource: map[string]any{"org_id": subject.OrgID, "branch_id": req.Header.Get("X-Branch-Id")},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "customer.card.manage")
			return
		}
		var body domain.CreateCustomerCardRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.CustomerID = id
		if body.IssuedBy == "" {
			body.IssuedBy = subject.Sub
		}
		card, err := inventoryStore.CreateCustomerCard(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, card)
	})

	r.Post("/customers/cards/{id}/block", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "customer.card.manage",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "customer.card.manage")
			return
		}
		var body domain.BlockCardRequest
		_ = json.NewDecoder(req.Body).Decode(&body)
		body.Actor = subject.Sub
		card, err := inventoryStore.BlockCustomerCard(req.Context(), subject.OrgID, id, body)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"block_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, card)
	})

	// —— POS / Caja ——
	r.Get("/pos/fiscal-settings", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.sale.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.sale.read")
			return
		}
		settings, err := inventoryStore.GetFiscalSettings(req.Context(), subject.OrgID, branchID)
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	r.Put("/pos/fiscal-settings", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.UpsertFiscalSettingsRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		if body.BranchID == "" {
			body.BranchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.settings.manage",
			Resource: map[string]any{"branch_id": body.BranchID, "org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.settings.manage")
			return
		}
		settings, err := inventoryStore.UpsertFiscalSettings(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"save_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	r.Get("/pos/sales", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.sale.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.sale.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		items, err := inventoryStore.ListSales(req.Context(), domain.SaleFilter{
			OrgRef: subject.OrgID, BranchCode: branchID, Limit: limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.POSSale{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	r.Get("/pos/sales/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.sale.read",
			Resource: map[string]any{"branch_id": req.Header.Get("X-Branch-Id"), "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.sale.read")
			return
		}
		sale, err := inventoryStore.GetSale(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, sale)
	})

	r.Post("/pos/sales/complete", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CompleteSaleRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.CashierSub = subject.Sub
		if body.BranchID == "" {
			body.BranchID = req.Header.Get("X-Branch-Id")
		}
		if body.OperatorLabel == "" {
			body.OperatorLabel = req.Header.Get("X-Operator-Label")
		}
		if body.SessionID == "" {
			body.SessionID = req.Header.Get("X-Session-Id")
		}
		if body.StationID == "" {
			body.StationID = req.Header.Get("X-Station-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.sale.create",
			Resource: map[string]any{"branch_id": body.BranchID, "org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.sale.create")
			return
		}
		sale, err := inventoryStore.CompleteSale(req.Context(), body)
		if errors.Is(err, domain.ErrInsufficientStock) {
			http.Error(w, `{"error":"insufficient_stock","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"sale_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, sale)
	})

	r.Post("/pos/sales/{id}/invoice", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		var body domain.RequestInvoiceRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.Actor = subject.Sub
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.invoice.request",
			Resource: map[string]any{"branch_id": req.Header.Get("X-Branch-Id"), "org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.invoice.request")
			return
		}
		inv, err := inventoryStore.RequestSaleInvoice(req.Context(), subject.OrgID, id, body)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"invoice_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, inv)
	})

	// —— Card payment wait queue (bank terminal) ——
	r.Get("/pos/card-waits", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.card.wait.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.card.wait.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		items, err := inventoryStore.ListCardPaymentWaits(req.Context(), domain.CardPaymentWaitFilter{
			OrgRef: subject.OrgID, BranchCode: branchID, Status: req.URL.Query().Get("status"), Limit: limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.CardPaymentWait{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	r.Get("/pos/card-waits/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.card.wait.read",
			Resource: map[string]any{"branch_id": req.Header.Get("X-Branch-Id"), "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.card.wait.read")
			return
		}
		item, err := inventoryStore.GetCardPaymentWait(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	r.Post("/pos/card-waits", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CreateCardPaymentWaitRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.CreatedBy = subject.Sub
		if body.BranchID == "" {
			body.BranchID = req.Header.Get("X-Branch-Id")
		}
		if body.OperatorLabel == "" {
			body.OperatorLabel = req.Header.Get("X-Operator-Label")
		}
		if body.StationID == "" {
			body.StationID = req.Header.Get("X-Station-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.card.wait.create",
			Resource: map[string]any{"branch_id": body.BranchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.card.wait.create")
			return
		}
		item, err := inventoryStore.CreateCardPaymentWait(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})

	r.Post("/pos/card-waits/{id}/confirm", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		var body domain.ConfirmCardPaymentWaitRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.ConfirmedBy = subject.Sub
		if body.OperatorLabel == "" {
			body.OperatorLabel = req.Header.Get("X-Operator-Label")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.card.wait.confirm",
			Resource: map[string]any{"branch_id": req.Header.Get("X-Branch-Id"), "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.card.wait.confirm")
			return
		}
		item, err := inventoryStore.ConfirmCardPaymentWait(req.Context(), subject.OrgID, id, body)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"confirm_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	r.Post("/pos/card-waits/{id}/cancel", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		var body domain.CancelCardPaymentWaitRequest
		_ = json.NewDecoder(req.Body).Decode(&body)
		body.CancelledBy = subject.Sub
		if body.OperatorLabel == "" {
			body.OperatorLabel = req.Header.Get("X-Operator-Label")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "pos.card.wait.cancel",
			Resource: map[string]any{"branch_id": req.Header.Get("X-Branch-Id"), "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "pos.card.wait.cancel")
			return
		}
		item, err := inventoryStore.CancelCardPaymentWait(req.Context(), subject.OrgID, id, body)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"cancel_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	// —— CEDI inbound trucks / tarimas ——
	r.Get("/inbound-shipments", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.URL.Query().Get("branch_id")
		if branchID == "" {
			branchID = req.Header.Get("X-Branch-Id")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.shipment.read",
			Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.shipment.read")
			return
		}
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		items, err := inventoryStore.ListInboundShipments(req.Context(), domain.InboundShipmentFilter{
			OrgRef: subject.OrgID, BranchCode: branchID, Status: req.URL.Query().Get("status"), Limit: limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.InboundShipment{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	r.Get("/inbound-shipments/{id}", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.shipment.read",
			Resource: map[string]any{"branch_id": req.Header.Get("X-Branch-Id"), "org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.shipment.read")
			return
		}
		ship, err := inventoryStore.GetInboundShipment(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, ship)
	})

	r.Post("/inbound-shipments", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CreateInboundShipmentRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.CreatedBy = subject.Sub
		if body.BranchID == "" {
			body.BranchID = req.Header.Get("X-Branch-Id")
		}
		if body.OperatorLabel == "" {
			body.OperatorLabel = req.Header.Get("X-Operator-Label")
		}
		if body.SessionID == "" {
			body.SessionID = req.Header.Get("X-Session-Id")
		}
		if body.IdempotencyKey == "" {
			body.IdempotencyKey = req.Header.Get("Idempotency-Key")
		}
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.shipment.create",
			Resource: map[string]any{
				"branch_id": body.BranchID, "warehouse_id": body.WarehouseID, "org_id": subject.OrgID,
			},
			Context: map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.shipment.create")
			return
		}
		ship, err := inventoryStore.CreateInboundShipment(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, ship)
	})

	r.Post("/inbound-shipments/{id}/post", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.shipment.post",
			Resource: map[string]any{"org_id": subject.OrgID, "branch_id": req.Header.Get("X-Branch-Id")},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.shipment.post")
			return
		}
		ship, err := inventoryStore.PostInboundShipment(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"post_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, ship)
	})

	r.Get("/security-logistics", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		branchID := req.Header.Get("X-Branch-Id")
		if q := req.URL.Query().Get("branch_id"); q != "" {
			branchID = q
		}
		kind := req.URL.Query().Get("kind")
		sealOnly := req.URL.Query().Get("seal_only") == "1" || strings.EqualFold(req.URL.Query().Get("seal_only"), "true")
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "reporting.security.read",
			Resource: map[string]any{
				"org_id":    subject.OrgID,
				"branch_id": branchID,
			},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "reporting.security.read")
			return
		}
		items, err := inventoryStore.ListSecurityLogistics(req.Context(), domain.SecurityLogisticsFilter{
			OrgRef:     subject.OrgID,
			BranchCode: branchID,
			Kind:       kind,
			SealOnly:   sealOnly,
			Limit:      limit,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.SecurityLogisticsEvent{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	r.Post("/transport-sheets/{id}/verify-seal", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		var body domain.VerifySealRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.Actor = subject.Sub
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.seal.verify",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.seal.verify")
			return
		}
		sheet, err := inventoryStore.VerifyTransportSeal(req.Context(), subject.OrgID, id, body)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"verify_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, sheet)
	})

	r.Post("/inbound-shipments/{id}/verify-seal", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		var body domain.VerifySealRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.OrgID = subject.OrgID
		body.Actor = subject.Sub
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "inventory.seal.verify",
			Resource: map[string]any{"org_id": subject.OrgID},
			Context:  map[string]any{"mfa_level": subject.MFALevel()},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "inventory.seal.verify")
			return
		}
		ship, err := inventoryStore.VerifyInboundSeal(req.Context(), subject.OrgID, id, body)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"verify_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, ship)
	})

	log.Printf("inventory listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

func customerIDFromSubject(subject authz.Subject) string {
	if subject.Attrs == nil {
		return ""
	}
	if v, ok := subject.Attrs["customer_id"].(string); ok {
		return v
	}
	return ""
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
