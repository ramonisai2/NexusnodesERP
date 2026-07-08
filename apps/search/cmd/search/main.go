package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/ramonisai2/NexusnodesERP/apps/search/internal/engine"
	"github.com/ramonisai2/NexusnodesERP/apps/search/internal/source"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
)

func main() {
	ctx := context.Background()
	shutdown, err := otelx.Init(ctx, "search")
	if err != nil {
		log.Fatalf("otel: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	idx := engine.NewIndex()
	src := source.NewPostgres(pool)
	svc := &server{
		idx: idx,
		src: src,
		opa: authz.NewClientFromEnv(),
	}

	intervalSec, _ := strconv.Atoi(envOr("SEARCH_REINDEX_SECONDS", "30"))
	if intervalSec < 5 {
		intervalSec = 5
	}
	if err := svc.reindex(ctx); err != nil {
		log.Printf("search initial reindex warning: %v", err)
	}
	go svc.loop(ctx, time.Duration(intervalSec)*time.Second)

	addr := envOr("SEARCH_ADDR", ":8086")
	r := chi.NewRouter()
	r.Use(otelx.Middleware("search"))
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		st := idx.Stats()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"service":    "search",
			"documents":  st.Documents,
			"terms":      st.Terms,
			"indexed_at": st.IndexedAt,
			"engine":     "inverted-index-bm25",
		})
	})
	// Gateway strips /search → so /search?q= arrives as /?q=
	r.Get("/", svc.search)
	r.Get("/stats", svc.stats)
	r.Post("/reindex", svc.reindexHandler)

	log.Printf("search listening on %s (BM25 inverted index)", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

type server struct {
	idx *engine.Index
	src *source.Postgres
	opa *authz.Client

	mu         sync.Mutex
	lastError  string
	lastReindex time.Time
}

func (s *server) loop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.reindex(ctx); err != nil {
				log.Printf("search reindex: %v", err)
			}
		}
	}
}

func (s *server) reindex(ctx context.Context) error {
	docs, err := s.src.LoadAll(ctx)
	if err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return err
	}
	s.idx.Rebuild(docs)
	s.mu.Lock()
	s.lastError = ""
	s.lastReindex = time.Now().UTC()
	s.mu.Unlock()
	return nil
}

func (s *server) search(w http.ResponseWriter, req *http.Request) {
	subject := authz.FromGatewayHeaders(req)
	q := strings.TrimSpace(req.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, `{"error":"q_required"}`, http.StatusBadRequest)
		return
	}
	branchID := req.URL.Query().Get("branch_id")
	if branchID == "" {
		branchID = req.Header.Get("X-Branch-Id")
	}
	if !s.authorize(w, req, subject, "search.query", branchID) {
		return
	}
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	kinds := parseKinds(req.URL.Query().Get("kind"))
	// Photos/labels are branch-scoped; products are org-wide (empty branch filter).
	branchFilter := branchID
	if onlyProducts(kinds) {
		branchFilter = ""
	}

	started := time.Now()
	hits := s.idx.Search(engine.Query{
		Text:     q,
		OrgID:    subject.OrgID,
		BranchID: branchFilter,
		Kinds:    kinds,
		Limit:    limit,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"q":       q,
		"took_ms": time.Since(started).Milliseconds(),
		"total":   len(hits),
		"hits":    hits,
		"engine":  "bm25",
	})
}

func (s *server) stats(w http.ResponseWriter, req *http.Request) {
	subject := authz.FromGatewayHeaders(req)
	if !s.authorize(w, req, subject, "search.query", "") {
		return
	}
	st := s.idx.Stats()
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"documents":    st.Documents,
		"terms":        st.Terms,
		"by_kind":      st.ByKind,
		"indexed_at":   st.IndexedAt,
		"last_reindex": s.lastReindex,
		"last_error":   s.lastError,
		"engine":       "inverted-index-bm25",
	})
}

func (s *server) reindexHandler(w http.ResponseWriter, req *http.Request) {
	subject := authz.FromGatewayHeaders(req)
	if !s.authorize(w, req, subject, "search.reindex", "") {
		return
	}
	if err := s.reindex(req.Context()); err != nil {
		http.Error(w, `{"error":"reindex_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stats": s.idx.Stats()})
}

func (s *server) authorize(w http.ResponseWriter, req *http.Request, subject authz.Subject, action, branchID string) bool {
	resource := map[string]any{"org_id": subject.OrgID}
	if branchID != "" {
		resource["branch_id"] = branchID
	}
	allow, err := s.opa.Allow(req.Context(), authz.Input{
		Subject:  subject,
		Action:   action,
		Resource: resource,
		Context:  map[string]any{"mfa_level": subject.MFALevel()},
	})
	if err != nil {
		http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
		return false
	}
	if !allow {
		authz.WriteForbidden(w, action)
		return false
	}
	return true
}

func parseKinds(raw string) []engine.Kind {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []engine.Kind
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(strings.ToLower(p))
		switch p {
		case "product", "label", "photo":
			out = append(out, engine.Kind(p))
		}
	}
	return out
}

func onlyProducts(kinds []engine.Kind) bool {
	if len(kinds) == 0 {
		return false
	}
	for _, k := range kinds {
		if k != engine.KindProduct {
			return false
		}
	}
	return true
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
