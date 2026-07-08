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
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func (s *Postgres) ListBalances(ctx context.Context, branchCode string) ([]domain.StockBalance, error) {
	q := `
SELECT sb.id::text, w.code, b.code, ps.sku, ps.sku,
       sb.on_hand::float8, sb.reserved::float8, sb.version
FROM stock_balances sb
JOIN warehouses w ON w.id = sb.warehouse_id
JOIN branches b ON b.id = w.branch_id
JOIN product_skus ps ON ps.id = sb.sku_id`
	args := []any{}
	if branchCode != "" {
		q += ` WHERE b.code = $1`
		args = append(args, branchCode)
	}
	q += ` ORDER BY b.code, w.code, ps.sku`

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.StockBalance
	for rows.Next() {
		var b domain.StockBalance
		if err := rows.Scan(&b.ID, &b.WarehouseID, &b.BranchID, &b.SKUID, &b.SKU, &b.OnHand, &b.Reserved, &b.Version); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Postgres) PostMovement(ctx context.Context, req domain.MovementRequest) (domain.Movement, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Movement{}, err
	}
	defer tx.Rollback(ctx)

	orgID, err := resolveOrgID(ctx, tx, req.OrgID)
	if err != nil {
		return domain.Movement{}, err
	}

	var existing domain.Movement
	err = tx.QueryRow(ctx, `
SELECT id::text, org_id::text, branch_id::text, warehouse_id::text, sku_id::text,
       movement_type, quantity::float8, status, COALESCE(posted_by::text, ''), idempotency_key, created_at
FROM inventory_movements WHERE org_id = $1 AND idempotency_key = $2`, orgID, req.IdempotencyKey).Scan(
		&existing.ID, &existing.OrgID, &existing.BranchID, &existing.WarehouseID, &existing.SKUID,
		&existing.MovementType, &existing.Quantity, &existing.Status, &existing.PostedBy, &existing.IdempotencyKey, &existing.CreatedAt,
	)
	if err == nil {
		// rewrite codes for response
		existing.BranchID = req.BranchID
		existing.WarehouseID = req.WarehouseID
		existing.SKUID = req.SKUID
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Movement{}, err
	}

	branchID, err := resolveBranchID(ctx, tx, orgID, req.BranchID)
	if err != nil {
		return domain.Movement{}, err
	}
	warehouseID, err := resolveWarehouseID(ctx, tx, branchID, req.WarehouseID)
	if err != nil {
		return domain.Movement{}, err
	}
	skuID, err := resolveSKUID(ctx, tx, req.SKUID)
	if err != nil {
		return domain.Movement{}, err
	}
	postedBy, err := resolveUserID(ctx, tx, orgID, req.PostedBy)
	if err != nil {
		return domain.Movement{}, err
	}

	delta, err := signedDelta(req.MovementType, req.Quantity)
	if err != nil {
		return domain.Movement{}, err
	}

	var balID string
	var onHand float64
	var version int
	err = tx.QueryRow(ctx, `
SELECT id::text, on_hand::float8, version
FROM stock_balances
WHERE warehouse_id = $1 AND sku_id = $2
FOR UPDATE`, warehouseID, skuID).Scan(&balID, &onHand, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Movement{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Movement{}, err
	}
	if req.ExpectedVersion != nil && version != *req.ExpectedVersion {
		return domain.Movement{}, domain.ErrConflict
	}

	next := onHand + delta
	if next < 0 {
		return domain.Movement{}, domain.ErrInsufficientStock
	}

	ct, err := tx.Exec(ctx, `
UPDATE stock_balances
SET on_hand = $1, version = version + 1, updated_at = now()
WHERE id = $2::uuid AND version = $3`, next, balID, version)
	if err != nil {
		return domain.Movement{}, err
	}
	if ct.RowsAffected() == 0 {
		return domain.Movement{}, domain.ErrConflict
	}

	movID := uuid.New()
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
INSERT INTO inventory_movements (
  id, org_id, branch_id, sku_id, warehouse_id, movement_type, quantity, status, posted_by, idempotency_key, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,'POSTED',$8,$9,$10)`,
		movID, orgID, branchID, skuID, warehouseID, strings.ToUpper(req.MovementType), delta, postedBy, req.IdempotencyKey, now)
	if err != nil {
		return domain.Movement{}, err
	}

	_, _ = tx.Exec(ctx, `
INSERT INTO outbox (event_type, payload) VALUES ('InventoryMoved', jsonb_build_object(
  'movement_id', $1::text,
  'branch_id', $2::text,
  'warehouse_id', $3::text,
  'sku_id', $4::text,
  'quantity', $5::float8
))`, movID.String(), req.BranchID, req.WarehouseID, req.SKUID, delta)

	if err := tx.Commit(ctx); err != nil {
		return domain.Movement{}, err
	}

	return domain.Movement{
		ID:             movID.String(),
		OrgID:          req.OrgID,
		BranchID:       req.BranchID,
		WarehouseID:    req.WarehouseID,
		SKUID:          req.SKUID,
		MovementType:   strings.ToUpper(req.MovementType),
		Quantity:       delta,
		Status:         "POSTED",
		PostedBy:       req.PostedBy,
		IdempotencyKey: req.IdempotencyKey,
		CreatedAt:      now,
	}, nil
}

func signedDelta(movementType string, qty float64) (float64, error) {
	if qty == 0 {
		return 0, errors.New("quantity required")
	}
	delta := qty
	switch strings.ToUpper(movementType) {
	case "RECEIPT", "ADJUST_IN", "TRANSFER_IN":
		if delta < 0 {
			delta = -delta
		}
	case "ISSUE", "ADJUST_OUT", "TRANSFER_OUT":
		if delta > 0 {
			delta = -delta
		}
	case "ADJUST":
	default:
		return 0, errors.New("invalid movement_type")
	}
	return delta, nil
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

func resolveWarehouseID(ctx context.Context, tx pgx.Tx, branchID, whRef string) (string, error) {
	if _, err := uuid.Parse(whRef); err == nil {
		return whRef, nil
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM warehouses WHERE branch_id = $1::uuid AND code = $2`, branchID, whRef).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("warehouse not found: %s", whRef)
	}
	return id, err
}

func resolveSKUID(ctx context.Context, tx pgx.Tx, skuRef string) (string, error) {
	if _, err := uuid.Parse(skuRef); err == nil {
		return skuRef, nil
	}
	// accept legacy aliases from early demo
	alias := map[string]string{"sku_bolt": "BOLT-M8", "sku_nut": "NUT-M8"}
	if v, ok := alias[skuRef]; ok {
		skuRef = v
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM product_skus WHERE sku = $1`, skuRef).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("sku not found: %s", skuRef)
	}
	return id, err
}

func resolveUserID(ctx context.Context, tx pgx.Tx, orgID, userRef string) (*string, error) {
	if userRef == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(userRef); err == nil {
		return &userRef, nil
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE org_id = $1::uuid AND idp_sub = $2`, orgID, userRef).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// fallback: any org by idp_sub
		err = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE idp_sub = $1`, userRef).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}
