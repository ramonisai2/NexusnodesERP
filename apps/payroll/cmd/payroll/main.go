package main

import (
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
	addr := envOr("PAYROLL_ADDR", ":8083")
	store := NewMemoryStore()
	_ = store.SeedDemo()

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "payroll"})
	})

	r.Get("/runs", func(w http.ResponseWriter, req *http.Request) {
		if !hasPerm(req, "payroll.run.read") {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		writeJSON(w, http.StatusOK, store.ListRuns())
	})

	r.Post("/runs", func(w http.ResponseWriter, req *http.Request) {
		if !hasPerm(req, "payroll.run.prepare") {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		var body CreateRunRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		body.PreparedBy = headerOr(req, "X-User-Id", "usr_unknown")
		if !branchAllowed(req, body.BranchID) {
			http.Error(w, `{"error":"branch_forbidden"}`, http.StatusForbidden)
			return
		}
		run, err := store.CreateAndCalculate(body)
		if err != nil {
			http.Error(w, `{"error":"create_failed","detail":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, run)
	})

	r.Post("/runs/{id}/approve", func(w http.ResponseWriter, req *http.Request) {
		if !hasPerm(req, "payroll.run.approve") {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		actor := headerOr(req, "X-User-Id", "usr_unknown")
		run, err := store.Approve(chi.URLParam(req, "id"), actor)
		if errors.Is(err, ErrSoDViolation) {
			http.Error(w, `{"error":"sod_violation"}`, http.StatusForbidden)
			return
		}
		if errors.Is(err, ErrInvalidState) {
			http.Error(w, `{"error":"invalid_state"}`, http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"approve_failed"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, run)
	})

	log.Printf("payroll listening on %s", addr)
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

func headerOr(r *http.Request, k, def string) string {
	if v := r.Header.Get(k); v != "" {
		return v
	}
	return def
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

var (
	ErrSoDViolation = errors.New("segregation of duties violation")
	ErrInvalidState = errors.New("invalid state")
	ErrNotFound     = errors.New("not found")
)

type Employee struct {
	ID         string  `json:"id"`
	BranchID   string  `json:"branch_id"`
	Name       string  `json:"name"`
	BaseSalary float64 `json:"base_salary"`
}

type PayrollLine struct {
	EmployeeID  string  `json:"employee_id"`
	ConceptCode string  `json:"concept_code"`
	Amount      float64 `json:"amount"`
}

type PayrollRun struct {
	ID          string        `json:"id"`
	PeriodLabel string        `json:"period_label"`
	BranchID    string        `json:"branch_id"`
	Status      string        `json:"status"`
	PreparedBy  string        `json:"prepared_by"`
	ApprovedBy  string        `json:"approved_by,omitempty"`
	TotalAmount float64       `json:"total_amount"`
	Lines       []PayrollLine `json:"lines"`
	Version     int           `json:"version"`
}

type CreateRunRequest struct {
	PeriodLabel string `json:"period_label"`
	BranchID    string `json:"branch_id"`
	PreparedBy  string `json:"prepared_by"`
}

type MemoryStore struct {
	mu        sync.Mutex
	employees []Employee
	runs      map[string]*PayrollRun
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{runs: map[string]*PayrollRun{}}
}

func (s *MemoryStore) SeedDemo() error {
	s.employees = []Employee{
		{ID: "emp_1", BranchID: "br_norte", Name: "Ana López", BaseSalary: 25000},
		{ID: "emp_2", BranchID: "br_norte", Name: "Luis Pérez", BaseSalary: 22000},
		{ID: "emp_3", BranchID: "br_sur", Name: "María Ruiz", BaseSalary: 28000},
	}
	return nil
}

func (s *MemoryStore) ListRuns() []PayrollRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PayrollRun, 0, len(s.runs))
	for _, r := range s.runs {
		out = append(out, *r)
	}
	return out
}

func (s *MemoryStore) CreateAndCalculate(req CreateRunRequest) (PayrollRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.BranchID == "" || req.PeriodLabel == "" {
		return PayrollRun{}, errors.New("branch_id and period_label required")
	}
	run := &PayrollRun{
		ID:          "run_" + uuid.NewString(),
		PeriodLabel: req.PeriodLabel,
		BranchID:    req.BranchID,
		Status:      "IN_REVIEW",
		PreparedBy:  req.PreparedBy,
		Version:     1,
	}
	var total float64
	for _, e := range s.employees {
		if e.BranchID != req.BranchID {
			continue
		}
		// Simple concept: BASE salary (demo calculation)
		line := PayrollLine{EmployeeID: e.ID, ConceptCode: "BASE", Amount: e.BaseSalary}
		run.Lines = append(run.Lines, line)
		total += e.BaseSalary
	}
	run.TotalAmount = total
	s.runs[run.ID] = run
	return *run, nil
}

func (s *MemoryStore) Approve(id, actor string) (PayrollRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return PayrollRun{}, ErrNotFound
	}
	if run.Status != "IN_REVIEW" {
		return PayrollRun{}, ErrInvalidState
	}
	if actor == run.PreparedBy {
		return PayrollRun{}, ErrSoDViolation
	}
	run.Status = "APPROVED"
	run.ApprovedBy = actor
	run.Version++
	return *run, nil
}
