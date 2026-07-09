package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/ramonisai2/NexusnodesERP/apps/messaging/internal/store"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
)

func main() {
	ctx := context.Background()
	shutdown, err := otelx.Init(ctx, "messaging")
	if err != nil {
		log.Fatalf("otel: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	svc := &server{
		store: store.New(pool),
		opa:   authz.NewClientFromEnv(),
	}

	addr := envOr("MESSAGING_ADDR", ":8087")
	r := chi.NewRouter()
	r.Use(otelx.Middleware("messaging"))
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "messaging"})
	})

	// Gateway strips /mail → paths arrive without the /mail prefix.
	r.Get("/inbox", svc.list("INBOX"))
	r.Get("/sent", svc.list("SENT"))
	r.Get("/archive", svc.list("ARCHIVE"))
	r.Get("/unread-count", svc.unread)
	r.Get("/directory", svc.directory)
	r.Get("/messages/{id}", svc.get)
	r.Post("/messages", svc.send)
	r.Post("/messages/{id}/read", svc.markRead)
	r.Post("/messages/{id}/archive", svc.archive)

	log.Printf("messaging listening on %s (mini-Outlook)", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

type server struct {
	store *store.Store
	opa   *authz.Client
}

func (s *server) authorize(w http.ResponseWriter, req *http.Request, subject authz.Subject, action string) bool {
	allow, err := s.opa.Allow(req.Context(), authz.Input{
		Subject:  subject,
		Action:   action,
		Resource: map[string]any{"org_id": subject.OrgID},
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

func (s *server) list(folder string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject := authz.FromGatewayHeaders(r)
		if !s.authorize(w, r, subject, "mail.read") {
			return
		}
		kind := strings.TrimSpace(r.URL.Query().Get("kind"))
		items, err := s.store.List(r.Context(), store.ListFilter{
			OrgRef: subject.OrgID,
			Sub:    subject.Sub,
			Folder: folder,
			Kind:   kind,
		})
		if err != nil {
			http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []store.Message{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "folder": folder})
	}
}

func (s *server) unread(w http.ResponseWriter, r *http.Request) {
	subject := authz.FromGatewayHeaders(r)
	if !s.authorize(w, r, subject, "mail.read") {
		return
	}
	n, err := s.store.UnreadCount(r.Context(), subject.OrgID, subject.Sub)
	if err != nil {
		http.Error(w, `{"error":"count_failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unread": n})
}

func (s *server) directory(w http.ResponseWriter, r *http.Request) {
	subject := authz.FromGatewayHeaders(r)
	if !s.authorize(w, r, subject, "mail.read") {
		return
	}
	items, err := s.store.Directory(r.Context(), subject.OrgID)
	if err != nil {
		http.Error(w, `{"error":"directory_failed"}`, http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []map[string]string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) get(w http.ResponseWriter, r *http.Request) {
	subject := authz.FromGatewayHeaders(r)
	if !s.authorize(w, r, subject, "mail.read") {
		return
	}
	id := chi.URLParam(r, "id")
	msg, err := s.store.Get(r.Context(), subject.OrgID, subject.Sub, id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"get_failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

type sendBody struct {
	Subject      string   `json:"subject"`
	Body         string   `json:"body"`
	Priority     string   `json:"priority"`
	Kind         string   `json:"kind"`
	ToSubs       []string `json:"to_subs"`
	RecipientIDs []string `json:"recipient_ids"` // alias
	ExpiresAt    *string  `json:"expires_at"`
	ColorToken   string   `json:"color_token"`
}

func (s *server) send(w http.ResponseWriter, r *http.Request) {
	subject := authz.FromGatewayHeaders(r)
	if !s.authorize(w, r, subject, "mail.send") {
		return
	}
	var body sendBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	kind := strings.ToUpper(strings.TrimSpace(body.Kind))
	if kind == "" {
		kind = store.KindMessage
	}
	if kind == store.KindAnnouncement {
		if !s.authorize(w, r, subject, "mail.announce") {
			return
		}
	}
	var expires *time.Time
	if body.ExpiresAt != nil && strings.TrimSpace(*body.ExpiresAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*body.ExpiresAt))
		if err != nil {
			http.Error(w, `{"error":"invalid_expires_at"}`, http.StatusBadRequest)
			return
		}
		expires = &t
	}
	to := body.ToSubs
	if len(to) == 0 {
		to = body.RecipientIDs
	}
	msg, err := s.store.Send(r.Context(), store.SendInput{
		OrgRef:       subject.OrgID,
		FromSub:      subject.Sub,
		FromOperator: subject.OperatorLabel,
		ToSubs:       to,
		Subject:      body.Subject,
		Body:         body.Body,
		Priority:     body.Priority,
		Kind:         kind,
		ExpiresAt:    expires,
		ColorToken:   body.ColorToken,
	})
	if err != nil {
		http.Error(w, `{"error":"send_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

func (s *server) markRead(w http.ResponseWriter, r *http.Request) {
	subject := authz.FromGatewayHeaders(r)
	if !s.authorize(w, r, subject, "mail.read") {
		return
	}
	id := chi.URLParam(r, "id")
	msg, err := s.store.MarkRead(r.Context(), subject.OrgID, subject.Sub, id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"mark_read_failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

func (s *server) archive(w http.ResponseWriter, r *http.Request) {
	subject := authz.FromGatewayHeaders(r)
	if !s.authorize(w, r, subject, "mail.read") {
		return
	}
	id := chi.URLParam(r, "id")
	msg, err := s.store.Archive(r.Context(), subject.OrgID, subject.Sub, id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"archive_failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, msg)
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
