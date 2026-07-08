package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/blob"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/imaging"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/store"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/uploadsession"
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
	sessionTTLMin, _ := strconv.Atoi(envOr("IMAGE_UPLOAD_SESSION_TTL_MIN", "20"))
	if sessionTTLMin <= 0 {
		sessionTTLMin = 20
	}

	svc := &server{
		pool:        pool,
		store:       store.NewPostgres(pool),
		sessions:    uploadsession.NewStore(pool),
		blobs:       blobs,
		opa:         authz.NewClientFromEnv(),
		maxEdge:     maxEdge,
		jpegQuality: jpegQuality,
		maxUpload:   int64(maxUpload),
		sessionTTL:  time.Duration(sessionTTLMin) * time.Minute,
		publicBase:  strings.TrimRight(envOr("PUBLIC_WEB_BASE", "http://localhost:5173"), "/"),
	}

	addr := envOr("REPORTS_ADDR", ":8085")
	r := chi.NewRouter()
	r.Use(otelx.Middleware("reports"))
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(90 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "reports"})
	})

	r.Get("/images", svc.listImages)
	r.Post("/images", svc.createImage)
	r.Get("/images/{id}", svc.getImage)
	r.Get("/images/{id}/content", svc.getImageContent)

	// QR mobile upload: create session (JWT via gateway) + public token endpoints.
	r.Post("/images/upload-sessions", svc.createUploadSession)
	r.Get("/images/upload/{token}", svc.getUploadSession)
	r.Post("/images/upload/{token}", svc.uploadViaToken)

	log.Printf("reports listening on %s storage=%s max_edge=%d jpeg_q=%d", addr, storageRoot, maxEdge, jpegQuality)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

type server struct {
	pool        *pgxpool.Pool
	store       domain.Store
	sessions    *uploadsession.Store
	blobs       *blob.LocalStore
	opa         *authz.Client
	maxEdge     int
	jpegQuality int
	maxUpload   int64
	sessionTTL  time.Duration
	publicBase  string
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

	rep, err := s.saveUploadedFile(req.Context(), subject.OrgID, branchID, subject.Sub, title, notes, file, header)
	if err != nil {
		writeUploadErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rep)
}

func (s *server) createUploadSession(w http.ResponseWriter, req *http.Request) {
	subject := authz.FromGatewayHeaders(req)
	var body struct {
		BranchID  string `json:"branch_id"`
		TitleHint string `json:"title_hint"`
		MaxFiles  int    `json:"max_files"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	if body.BranchID == "" {
		body.BranchID = req.Header.Get("X-Branch-Id")
	}
	if body.BranchID == "" {
		http.Error(w, `{"error":"branch_id_required"}`, http.StatusBadRequest)
		return
	}
	if !s.authorize(w, req, subject, "reporting.image.create", body.BranchID) {
		return
	}
	orgUUID, err := resolveOrgUUID(req.Context(), s.pool, subject.OrgID)
	if err != nil {
		http.Error(w, `{"error":"org_resolve_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
		return
	}
	raw, sess, err := s.sessions.Create(req.Context(), orgUUID, body.BranchID, subject.Sub, strings.TrimSpace(body.TitleHint), s.sessionTTL, body.MaxFiles)
	if err != nil {
		http.Error(w, `{"error":"session_create_failed","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      raw,
		"expires_at": sess.ExpiresAt,
		"max_files":  sess.MaxFiles,
		"remaining":  sess.Remaining,
		"branch_id":  sess.BranchID,
		"title_hint": sess.TitleHint,
		"upload_path": "/upload/" + raw,
		"upload_url":  s.publicBase + "/upload/" + raw,
		"api_upload":  "/reports/images/upload/" + raw,
	})
}

func (s *server) getUploadSession(w http.ResponseWriter, req *http.Request) {
	token := chi.URLParam(req, "token")
	sess, err := s.sessions.GetByToken(req.Context(), token)
	if err != nil {
		writeSessionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"expires_at": sess.ExpiresAt,
		"max_files":  sess.MaxFiles,
		"remaining":  sess.Remaining,
		"title_hint": sess.TitleHint,
		"branch_id":  sess.BranchID,
	})
}

func (s *server) uploadViaToken(w http.ResponseWriter, req *http.Request) {
	token := chi.URLParam(req, "token")
	if err := req.ParseMultipartForm(s.maxUpload * 8); err != nil {
		http.Error(w, `{"error":"invalid_multipart","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
		return
	}

	// Collect files: support "file", "files", and "files[]"
	var headers []*multipart.FileHeader
	if req.MultipartForm != nil {
		for _, key := range []string{"file", "files", "files[]"} {
			headers = append(headers, req.MultipartForm.File[key]...)
		}
	}
	if len(headers) == 0 {
		http.Error(w, `{"error":"file_required"}`, http.StatusBadRequest)
		return
	}
	if len(headers) > 12 {
		http.Error(w, `{"error":"too_many_files"}`, http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(req.FormValue("title"))
	notes := strings.TrimSpace(req.FormValue("notes"))

	sess, err := s.sessions.ClaimUpload(req.Context(), token, len(headers))
	if err != nil {
		writeSessionErr(w, err)
		return
	}
	if title == "" {
		if sess.TitleHint != "" {
			title = sess.TitleHint
		} else {
			title = "Reporte desde teléfono"
		}
	}

	created := make([]domain.ImageReport, 0, len(headers))
	for i, fh := range headers {
		f, err := fh.Open()
		if err != nil {
			http.Error(w, `{"error":"read_failed"}`, http.StatusBadRequest)
			return
		}
		itemTitle := title
		if len(headers) > 1 {
			itemTitle = fmt.Sprintf("%s (%d/%d)", title, i+1, len(headers))
		}
		rep, err := s.saveUploadedFile(req.Context(), sess.OrgID, sess.BranchID, sess.CreatedBy, itemTitle, notes, f, fh)
		_ = f.Close()
		if err != nil {
			writeUploadErr(w, err)
			return
		}
		created = append(created, rep)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"uploaded":  len(created),
		"remaining": sess.Remaining,
		"items":     created,
	})
}

func (s *server) saveUploadedFile(
	ctx context.Context,
	orgID, branchID, createdBy, title, notes string,
	file multipart.File,
	header *multipart.FileHeader,
) (domain.ImageReport, error) {
	ct := header.Header.Get("Content-Type")
	if !imaging.IsAllowedContentType(ct) {
		return domain.ImageReport{}, errBad("unsupported_content_type")
	}
	limited := io.LimitReader(file, s.maxUpload+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return domain.ImageReport{}, errBad("read_failed")
	}
	if int64(len(raw)) > s.maxUpload {
		return domain.ImageReport{}, errBad("file_too_large")
	}
	processed, err := imaging.Kamehameha(bytes.NewReader(raw), imaging.Options{
		MaxEdge:     s.maxEdge,
		JPEGQuality: s.jpegQuality,
	})
	if err != nil {
		return domain.ImageReport{}, fmt.Errorf("image_process_failed: %w", err)
	}
	id := uuid.NewString()
	key := path.Join(sanitizePath(orgID), sanitizePath(branchID), id+".jpg")
	if err := s.blobs.Put(key, processed.Bytes); err != nil {
		return domain.ImageReport{}, fmt.Errorf("store_failed: %w", err)
	}
	rep, err := s.store.Create(ctx, domain.CreateImageReport{
		OrgID:            orgID,
		BranchID:         branchID,
		CreatedBy:        createdBy,
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
		return domain.ImageReport{}, fmt.Errorf("persist_failed: %w", err)
	}
	rep.ContentURL = "/reports/images/" + rep.ID + "/content"
	return rep, nil
}

type badRequestError struct{ code string }

func (e badRequestError) Error() string { return e.code }
func errBad(code string) error          { return badRequestError{code: code} }

func writeUploadErr(w http.ResponseWriter, err error) {
	var br badRequestError
	if errors.As(err, &br) {
		status := http.StatusBadRequest
		if br.code == "file_too_large" {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, `{"error":"`+br.code+`"}`, status)
		return
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "image_process_failed") {
		http.Error(w, `{"error":"image_process_failed","detail":"`+escapeJSON(msg)+`"}`, http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(msg, "persist_failed") {
		http.Error(w, `{"error":"persist_failed","detail":"`+escapeJSON(msg)+`"}`, http.StatusBadRequest)
		return
	}
	http.Error(w, `{"error":"upload_failed","detail":"`+escapeJSON(msg)+`"}`, http.StatusInternalServerError)
}

func writeSessionErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, uploadsession.ErrNotFound):
		http.Error(w, `{"error":"session_not_found"}`, http.StatusNotFound)
	case errors.Is(err, uploadsession.ErrExpired):
		http.Error(w, `{"error":"session_expired"}`, http.StatusGone)
	case errors.Is(err, uploadsession.ErrRevoked):
		http.Error(w, `{"error":"session_revoked"}`, http.StatusGone)
	case errors.Is(err, uploadsession.ErrExhausted):
		http.Error(w, `{"error":"session_exhausted"}`, http.StatusConflict)
	default:
		http.Error(w, `{"error":"session_error","detail":"`+escapeJSON(err.Error())+`"}`, http.StatusBadRequest)
	}
}

func resolveOrgUUID(ctx context.Context, pool *pgxpool.Pool, orgRef string) (string, error) {
	if orgRef == "" {
		return "", errors.New("org_id required")
	}
	if _, err := uuid.Parse(orgRef); err == nil {
		return orgRef, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err := db.SetRLSBypass(ctx, tx, true); err != nil {
		return "", err
	}
	code := orgRef
	if code == "org_demo" {
		code = "DEMO"
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM organizations WHERE code = $1`, code).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("org not found")
	}
	if err != nil {
		return "", err
	}
	_ = tx.Commit(ctx)
	return id, nil
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
