package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
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
		balances, err := inventoryStore.ListBalances(req.Context(), subject.OrgID, branchID)
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, balances)
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

		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "inventory.movement.create",
			Resource: map[string]any{
				"branch_id": body.BranchID,
				"org_id":    body.OrgID,
				"quantity":  body.Quantity,
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
