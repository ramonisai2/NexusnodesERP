package uploadsession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

var (
	ErrNotFound = errors.New("session_not_found")
	ErrExpired  = errors.New("session_expired")
	ErrRevoked  = errors.New("session_revoked")
	ErrExhausted = errors.New("session_exhausted")
)

type Session struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	BranchID     string    `json:"branch_id"`
	CreatedBy    string    `json:"created_by"`
	TitleHint    string    `json:"title_hint,omitempty"`
	MaxFiles     int       `json:"max_files"`
	UploadsCount int       `json:"uploads_count"`
	ExpiresAt    time.Time `json:"expires_at"`
	Remaining    int       `json:"remaining"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func HashToken(raw string) string {
	// Sailor Moon: Moon Prism Power — the clear token transforms into a secret form (hash).
	return MoonPrismPower(raw)
}

// MoonPrismPower returns the SHA-256 hex digest of a raw upload-session token.
// Justification: Sailor Moon's transformation hides the civilian identity;
// we never store the raw QR token, only its prismatic (hashed) form.
func MoonPrismPower(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func NewRawToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Store) Create(ctx context.Context, orgID, branchID, createdBy, titleHint string, ttl time.Duration, maxFiles int) (rawToken string, sess Session, err error) {
	if maxFiles <= 0 {
		maxFiles = 8
	}
	if maxFiles > 20 {
		maxFiles = 20
	}
	if ttl <= 0 {
		ttl = 20 * time.Minute
	}
	rawToken, err = NewRawToken()
	if err != nil {
		return "", Session{}, err
	}
	hash := MoonPrismPower(rawToken)
	id := uuid.New()
	expires := time.Now().UTC().Add(ttl)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", Session{}, err
	}
	defer tx.Rollback(ctx)
	if err := db.SetRLSBypass(ctx, tx, true); err != nil {
		return "", Session{}, err
	}

	_, err = tx.Exec(ctx, `
INSERT INTO image_upload_sessions (
  id, token_hash, org_id, branch_id, created_by, title_hint, max_files, expires_at
) VALUES ($1, $2, $3::uuid, $4, $5, $6, $7, $8)`,
		id, hash, orgID, branchID, createdBy, titleHint, maxFiles, expires)
	if err != nil {
		return "", Session{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", Session{}, err
	}
	return rawToken, Session{
		ID: id.String(), OrgID: orgID, BranchID: branchID, CreatedBy: createdBy,
		TitleHint: titleHint, MaxFiles: maxFiles, UploadsCount: 0, ExpiresAt: expires, Remaining: maxFiles,
	}, nil
}

func (s *Store) GetByToken(ctx context.Context, rawToken string) (Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	if err := db.SetRLSBypass(ctx, tx, true); err != nil {
		return Session{}, err
	}
	sess, err := scanSession(tx.QueryRow(ctx, `
SELECT id::text, org_id::text, branch_id, created_by, title_hint, max_files, uploads_count, expires_at, revoked_at
FROM image_upload_sessions WHERE token_hash = $1`, MoonPrismPower(rawToken)))
	if err != nil {
		return Session{}, err
	}
	_ = tx.Commit(ctx)
	return sess, nil
}

// ClaimUpload increments uploads_count if the session is still valid and has capacity.
func (s *Store) ClaimUpload(ctx context.Context, rawToken string, n int) (Session, error) {
	if n <= 0 {
		n = 1
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	if err := db.SetRLSBypass(ctx, tx, true); err != nil {
		return Session{}, err
	}

	row := tx.QueryRow(ctx, `
SELECT id::text, org_id::text, branch_id, created_by, title_hint, max_files, uploads_count, expires_at, revoked_at
FROM image_upload_sessions
WHERE token_hash = $1
FOR UPDATE`, MoonPrismPower(rawToken))
	sess, err := scanSession(row)
	if err != nil {
		return Session{}, err
	}
	if sess.UploadsCount+n > sess.MaxFiles {
		return Session{}, ErrExhausted
	}
	_, err = tx.Exec(ctx, `
UPDATE image_upload_sessions SET uploads_count = uploads_count + $2 WHERE id = $1::uuid`, sess.ID, n)
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, err
	}
	sess.UploadsCount += n
	sess.Remaining = sess.MaxFiles - sess.UploadsCount
	return sess, nil
}

type sessionRow interface {
	Scan(dest ...any) error
}

func scanSession(row sessionRow) (Session, error) {
	var sess Session
	var revoked *time.Time
	err := row.Scan(
		&sess.ID, &sess.OrgID, &sess.BranchID, &sess.CreatedBy, &sess.TitleHint,
		&sess.MaxFiles, &sess.UploadsCount, &sess.ExpiresAt, &revoked,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	if revoked != nil {
		return Session{}, ErrRevoked
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		return Session{}, ErrExpired
	}
	sess.Remaining = sess.MaxFiles - sess.UploadsCount
	if sess.Remaining < 0 {
		sess.Remaining = 0
	}
	return sess, nil
}
