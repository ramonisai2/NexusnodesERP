package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/blob"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/imaging"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/store"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
)

func main() {
	ctx := context.Background()
	shutdown, err := otelx.Init(ctx, "reports")
	if err != nil {
		log.Fatalf("otel: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	storageRoot := envOr("IMAGE_STORAGE_PATH", "./data/image-reports")
	blobs, err := blob.NewLocalStore(storageRoot)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	maxEdge, _ := strconv.Atoi(envOr("IMAGE_MAX_EDGE", strconv.Itoa(imaging.DefaultMaxEdge)))
	jpegQuality, _ := strconv.Atoi(envOr("IMAGE_JPEG_QUALITY", strconv.Itoa(imaging.DefaultJPEGQuality)))
	maxUpload, _ := strconv.Atoi(envOr("IMAGE_MAX_UPLOAD_BYTES", strconv.Itoa(imaging.DefaultMaxUpload)))
	if maxUpload <= 0 {
		maxUpload = imaging.DefaultMaxUpload
	}

	svc := &server{
		store:       store.NewPostgres(pool),
		blobs:       blobs,
		opa:         authz.NewClientFromEnv(),
		maxEdge:     maxEdge,
		jpegQuality: jpegQuality,
		maxUpload:   int64(maxUpload),
	}

	addr := envOr("REPORTS_ADDR", ":8085")
	r := chi.NewRouter()
	r.Use(otelx.Middleware("reports"))
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "reports"})
	})

	r.Get("/images", svc.listImages)
	r.Post("/images", svc.createImage)
	r.Get("/images/{id}", svc.getImage)
	r.Get("/images/{id}/content", svc.getImageContent)

	log.Printf("reports listening on %s storage=%s max_edge=%d jpeg_q=%d", addr, storageRoot, maxEdge, jpegQuality)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

type server struct {
	store       domain.Store
	blobs       *blob.LocalStore
	opa         *authz.Client
	maxEdge     int
	jpegQuality int
	maxUpload   int64
}

func (s *server) listImages(w http.ResponseWriter, req *http.Request) {
	subject := authz.FromGatewayHeaders(req)
	branchID := req.URL.Query().Get("branch_id")
	if branchID == "" {
		branchID = req.Header.Get("X-Branch-Id")
	}
	if branchID == "" {
		http.Error(w, `{"error":"branch_id_required"}`, http.StatusBadRequest)
		return
	}
	if !s.authorize(w, req, subject, "reporting.image.read", branchID) {
		return
	}
	items, err := s.store.List(req.Context(), subject.OrgID, branchID)
	if err != nil {
		http.Error(w, `{"error":"list_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
		return
	}
	for i := range items {
		items[i].ContentURL = "/reports/images/" + items[i].ID + "/content"
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) getImage(w http.ResponseWriter, req *http.Request) {
	subject := authz.FromGatewayHeaders(req)
	id := chi.URLParam(req, "id")
	rep, err := s.store.Get(req.Context(), subject.OrgID, id)
	if errors.Is(err, domain.ErrNotFound) {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"lookup_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
		return
	}
	if !s.authorize(w, req, subject, "reporting.image.read", rep.BranchID) {
		return
	}
	rep.ContentURL = "/reports/images/" + rep.ID + "/content"
	writeJSON(w, http.StatusOK, rep)
}

func (s *server) getImageContent(w http.ResponseWriter, req *http.Request) {
	subject := authz.FromGatewayHeaders(req)
	id := chi.URLParam(req, "id")
	rep, err := s.store.Get(req.Context(), subject.OrgID, id)
	if errors.Is(err, domain.ErrNotFound) {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"lookup_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
		return
	}
	if !s.authorize(w, req, subject, "reporting.image.read", rep.BranchID) {
		return
	}
	data, err := s.blobs.Get(rep.StorageKey)
	if err != nil {
		http.Error(w, `{"error":"blob_missing"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", rep.MimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *server) createImage(w http.ResponseWriter, req *http.Request) {
	subject := authz.FromGatewayHeaders(req)
	if err := req.ParseMultipartForm(s.maxUpload); err != nil {
		http.Error(w, `{"error":"invalid_multipart","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
		return
	}
	branchID := req.FormValue("branch_id")
	if branchID == "" {
		branchID = req.Header.Get("X-Branch-Id")
	}
	title := strings.TrimSpace(req.FormValue("title"))
	notes := strings.TrimSpace(req.FormValue("notes"))
	if branchID == "" || title == "" {
		http.Error(w, `{"error":"title_and_branch_required"}`, http.StatusBadRequest)
		return
	}
	if !s.authorize(w, req, subject, "reporting.image.create", branchID) {
		return
	}

	file, header, err := req.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file_required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	ct := header.Header.Get("Content-Type")
	if !imaging.IsAllowedContentType(ct) {
		http.Error(w, `{"error":"unsupported_content_type"}`, http.StatusBadRequest)
		return
	}
	limited := io.LimitReader(file, s.maxUpload+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, `{"error":"read_failed"}`, http.StatusBadRequest)
		return
	}
	if int64(len(raw)) > s.maxUpload {
		http.Error(w, `{"error":"file_too_large"}`, http.StatusRequestEntityTooLarge)
		return
	}

	processed, err := imaging.Process(bytes.NewReader(raw), imaging.Options{
		MaxEdge:     s.maxEdge,
		JPEGQuality: s.jpegQuality,
	})
	if err != nil {
		http.Error(w, `{"error":"image_process_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
		return
	}

	id := uuid.NewString()
	key := path.Join(sanitizePath(subject.OrgID), sanitizePath(branchID), id+".jpg")
	if err := s.blobs.Put(key, processed.Bytes); err != nil {
		http.Error(w, `{"error":"store_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
		return
	}

	rep, err := s.store.Create(req.Context(), domain.CreateImageReport{
		OrgID:            subject.OrgID,
		BranchID:         branchID,
		CreatedBy:        subject.Sub,
		Title:            title,
		Notes:            notes,
		StorageKey:       key,
		OriginalFilename: header.Filename,
		MimeType:         processed.MimeType,
		Width:            processed.Width,
		Height:           processed.Height,
		ByteSize:         len(processed.Bytes),
		OriginalWidth:    processed.OriginalWidth,
		OriginalHeight:   processed.OriginalHeight,
		OriginalByteSize: len(raw),
	})
	if err != nil {
		http.Error(w, `{"error":"persist_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
		return
	}
	rep.ContentURL = "/reports/images/" + rep.ID + "/content"
	writeJSON(w, http.StatusCreated, rep)
}

func (s *server) authorize(w http.ResponseWriter, req *http.Request, subject authz.Subject, action, branchID string) bool {
	allow, err := s.opa.Allow(req.Context(), authz.Input{
		Subject:  subject,
		Action:   action,
		Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
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

func sanitizePath(v string) string {
	v = strings.ReplaceAll(v, "..", "")
	v = strings.ReplaceAll(v, "/", "_")
	v = strings.ReplaceAll(v, "\\", "_")
	if v == "" {
		return "unknown"
	}
	return v
}

func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return s
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
