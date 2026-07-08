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
	"github.com/ramonisai2/NexusnodesERP/apps/payroll/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/apps/payroll/internal/store"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

func main() {
	addr := envOr("PAYROLL_ADDR", ":8083")
	ctx := context.Background()

	var payrollStore domain.Store
	if os.Getenv("DATABASE_URL") != "" {
		pool, err := db.Connect(ctx)
		if err != nil {
			log.Fatalf("postgres: %v", err)
		}
		payrollStore = store.NewPostgres(pool)
		log.Printf("payroll store=postgres")
	} else {
		mem := store.NewMemory()
		mem.SeedDemo()
		payrollStore = mem
		log.Printf("payroll store=memory")
	}

	opa := authz.NewClientFromEnv()

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "payroll"})
	})

	r.Get("/runs", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject:  subject,
			Action:   "payroll.run.read",
			Resource: map[string]any{"org_id": subject.OrgID},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "payroll.run.read")
			return
		}
		runs, err := payrollStore.ListRuns(req.Context(), subject.OrgID)
		if err != nil {
			http.Error(w, `{"error":"list_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, runs)
	})

	r.Post("/runs", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body domain.CreateRunRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		if body.PreparedBy == "" {
			body.PreparedBy = subject.Sub
		}
		if body.OrgID == "" {
			body.OrgID = subject.OrgID
		}

		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "payroll.run.prepare",
			Resource: map[string]any{
				"branch_id": body.BranchID,
				"org_id":    body.OrgID,
			},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "payroll.run.prepare")
			return
		}

		run, err := payrollStore.CreateAndCalculate(req.Context(), body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, run)
	})

	r.Post("/runs/{id}/approve", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		id := chi.URLParam(req, "id")
		existing, err := payrollStore.GetRun(req.Context(), subject.OrgID, id)
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"lookup_failed","detail":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		allow, err := opa.Allow(req.Context(), authz.Input{
			Subject: subject,
			Action:  "payroll.run.approve",
			Resource: map[string]any{
				"branch_id":    existing.BranchID,
				"org_id":       subject.OrgID,
				"prepared_by":  existing.PreparedBy,
				"total_amount": existing.TotalAmount,
			},
		})
		if err != nil {
			http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		if !allow {
			authz.WriteForbidden(w, "payroll.run.approve")
			return
		}

		run, err := payrollStore.Approve(req.Context(), subject.OrgID, id, subject.Sub)
		if errors.Is(err, domain.ErrSoDViolation) {
			http.Error(w, `{"error":"sod_violation"}`, http.StatusForbidden)
			return
		}
		if errors.Is(err, domain.ErrInvalidState) {
			http.Error(w, `{"error":"invalid_state"}`, http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"approve_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, run)
	})

	log.Printf("payroll listening on %s", addr)
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
