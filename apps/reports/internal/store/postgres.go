package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/apps/reports/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func (s *Postgres) Create(ctx context.Context, in domain.CreateImageReport) (domain.ImageReport, error) {
	if strings.TrimSpace(in.Title) == "" || in.StorageKey == "" || in.MimeType == "" {
		return domain.ImageReport{}, domain.ErrInvalid
	}
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, in.OrgID)
	})
	if err != nil {
		return domain.ImageReport{}, err
	}
	defer tx.Rollback(ctx)

	branchID, err := resolveBranchID(ctx, tx, orgID, in.BranchID)
	if err != nil {
		return domain.ImageReport{}, err
	}

	id := uuid.New()
	now := time.Now().UTC()
	var ow, oh, obs any
	if in.OriginalWidth > 0 {
		ow = in.OriginalWidth
	}
	if in.OriginalHeight > 0 {
		oh = in.OriginalHeight
	}
	if in.OriginalByteSize > 0 {
		obs = in.OriginalByteSize
	}

	_, err = tx.Exec(ctx, `
INSERT INTO image_reports (
  id, org_id, branch_id, created_by, title, notes, storage_key, original_filename,
  mime_type, width, height, byte_size, original_width, original_height, original_byte_size, created_at
) VALUES (
  $1, $2::uuid, $3::uuid, $4, $5, $6, $7, $8,
  $9, $10, $11, $12, $13, $14, $15, $16
)`,
		id, orgID, branchID, in.CreatedBy, strings.TrimSpace(in.Title), strings.TrimSpace(in.Notes),
		in.StorageKey, in.OriginalFilename, in.MimeType, in.Width, in.Height, in.ByteSize,
		ow, oh, obs, now)
	if err != nil {
		return domain.ImageReport{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.ImageReport{}, err
	}

	rep := domain.ImageReport{
		ID:               id.String(),
		OrgID:            orgID,
		BranchID:         in.BranchID,
		CreatedBy:        in.CreatedBy,
		Title:            strings.TrimSpace(in.Title),
		Notes:            strings.TrimSpace(in.Notes),
		StorageKey:       in.StorageKey,
		OriginalFilename: in.OriginalFilename,
		MimeType:         in.MimeType,
		Width:            in.Width,
		Height:           in.Height,
		ByteSize:         in.ByteSize,
		CreatedAt:        now,
	}
	if in.OriginalWidth > 0 {
		v := in.OriginalWidth
		rep.OriginalWidth = &v
	}
	if in.OriginalHeight > 0 {
		v := in.OriginalHeight
		rep.OriginalHeight = &v
	}
	if in.OriginalByteSize > 0 {
		v := in.OriginalByteSize
		rep.OriginalByteSize = &v
	}
	return rep, nil
}

func (s *Postgres) List(ctx context.Context, orgRef, branchRef string) ([]domain.ImageReport, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	branchID, err := resolveBranchID(ctx, tx, orgID, branchRef)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
SELECT r.id::text, r.org_id::text, COALESCE(b.code, r.branch_id::text), r.created_by, r.title, r.notes,
       r.storage_key, r.original_filename, r.mime_type, r.width, r.height, r.byte_size,
       r.original_width, r.original_height, r.original_byte_size, r.created_at
FROM image_reports r
LEFT JOIN branches b ON b.id = r.branch_id
WHERE r.org_id = $1::uuid AND r.branch_id = $2::uuid
ORDER BY r.created_at DESC
LIMIT 100`, orgID, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.ImageReport
	for rows.Next() {
		rep, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

func (s *Postgres) Get(ctx context.Context, orgRef, id string) (domain.ImageReport, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.ImageReport{}, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
SELECT r.id::text, r.org_id::text, COALESCE(b.code, r.branch_id::text), r.created_by, r.title, r.notes,
       r.storage_key, r.original_filename, r.mime_type, r.width, r.height, r.byte_size,
       r.original_width, r.original_height, r.original_byte_size, r.created_at
FROM image_reports r
LEFT JOIN branches b ON b.id = r.branch_id
WHERE r.org_id = $1::uuid AND r.id = $2::uuid`, orgID, id)

	rep, err := scanReport(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ImageReport{}, domain.ErrNotFound
	}
	return rep, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanReport(row scannable) (domain.ImageReport, error) {
	var rep domain.ImageReport
	var ow, oh, obs *int
	err := row.Scan(
		&rep.ID, &rep.OrgID, &rep.BranchID, &rep.CreatedBy, &rep.Title, &rep.Notes,
		&rep.StorageKey, &rep.OriginalFilename, &rep.MimeType, &rep.Width, &rep.Height, &rep.ByteSize,
		&ow, &oh, &obs, &rep.CreatedAt,
	)
	if err != nil {
		return domain.ImageReport{}, err
	}
	rep.OriginalWidth = ow
	rep.OriginalHeight = oh
	rep.OriginalByteSize = obs
	return rep, nil
}

func resolveOrgID(ctx context.Context, tx pgx.Tx, orgRef string) (string, error) {
	if orgRef == "" {
		return "", fmt.Errorf("org_id required")
	}
	if _, err := uuid.Parse(orgRef); err == nil {
		return orgRef, nil
	}
	code := orgRef
	if code == "org_demo" {
		code = "DEMO"
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM organizations WHERE code = $1`, code).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return id, err
}

func resolveBranchID(ctx context.Context, tx pgx.Tx, orgID, branchRef string) (string, error) {
	if _, err := uuid.Parse(branchRef); err == nil {
		return branchRef, nil
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM branches WHERE org_id = $1::uuid AND code = $2`, orgID, branchRef).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("branch not found: %s", branchRef)
	}
	return id, err
}
