package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

func (s *Postgres) CreateShippingSlip(ctx context.Context, req domain.CreateShippingSlipRequest) (domain.ShippingSlip, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return domain.ShippingSlip{}, errors.New("idempotency_key required")
	}
	if !domain.ValidContainerType(req.ContainerType) {
		return domain.ShippingSlip{}, fmt.Errorf("invalid container_type")
	}
	if strings.TrimSpace(req.FromBranchID) == "" || strings.TrimSpace(req.ToBranchID) == "" {
		return domain.ShippingSlip{}, errors.New("from_branch_id and to_branch_id required")
	}
	if req.FromBranchID == req.ToBranchID {
		return domain.ShippingSlip{}, errors.New("from and to branch must differ")
	}
	if strings.TrimSpace(req.Description) == "" && strings.TrimSpace(req.ContentsSummary) == "" && len(req.Lines) == 0 {
		return domain.ShippingSlip{}, errors.New("description, contents_summary, or lines required")
	}
	if req.QuantityUnits <= 0 {
		req.QuantityUnits = 1
	}

	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, req.OrgID)
	})
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	defer tx.Rollback(ctx)

	var existingID string
	err = tx.QueryRow(ctx, `
SELECT id::text FROM shipping_slips WHERE org_id = $1 AND idempotency_key = $2`,
		orgID, req.IdempotencyKey).Scan(&existingID)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return domain.ShippingSlip{}, err
		}
		return s.GetShippingSlip(ctx, req.OrgID, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.ShippingSlip{}, err
	}

	fromBranch, err := resolveBranchID(ctx, tx, orgID, req.FromBranchID)
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	toBranch, err := resolveBranchID(ctx, tx, orgID, req.ToBranchID)
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	var fromWH, toWH any
	if req.FromWarehouseID != "" {
		id, err := resolveWarehouseID(ctx, tx, fromBranch, req.FromWarehouseID)
		if err != nil {
			return domain.ShippingSlip{}, err
		}
		fromWH = id
	}
	if req.ToWarehouseID != "" {
		id, err := resolveWarehouseID(ctx, tx, toBranch, req.ToWarehouseID)
		if err != nil {
			return domain.ShippingSlip{}, err
		}
		toWH = id
	}
	var sess any
	if req.SessionID != "" {
		if _, err := uuid.Parse(req.SessionID); err == nil {
			sess = req.SessionID
		}
	}

	id := uuid.New()
	now := time.Now().UTC()
	slipNumber := fmt.Sprintf("PAP-%s", strings.ToUpper(id.String()[:8]))

	_, err = tx.Exec(ctx, `
INSERT INTO shipping_slips (
  id, org_id, slip_number, from_branch_id, to_branch_id, from_warehouse_id, to_warehouse_id,
  container_type, description, contents_summary, quantity_units, status,
  created_by, operator_label, session_id, notes, idempotency_key, created_at, updated_at
) VALUES (
  $1, $2::uuid, $3, $4::uuid, $5::uuid, $6, $7,
  $8, $9, $10, $11, 'DRAFT',
  $12, $13, $14, $15, $16, $17, $17
)`, id, orgID, slipNumber, fromBranch, toBranch, fromWH, toWH,
		strings.ToUpper(req.ContainerType), strings.TrimSpace(req.Description),
		strings.TrimSpace(req.ContentsSummary), req.QuantityUnits,
		req.CreatedBy, strings.TrimSpace(req.OperatorLabel), sess,
		strings.TrimSpace(req.Notes), req.IdempotencyKey, now)
	if err != nil {
		return domain.ShippingSlip{}, err
	}

	for i, line := range req.Lines {
		skuCode := strings.TrimSpace(line.SKU)
		desc := strings.TrimSpace(line.Description)
		qty := line.Quantity
		if qty <= 0 {
			qty = 1
		}
		var skuID any
		if skuCode != "" {
			if sid, err := resolveSKUID(ctx, tx, skuCode); err == nil {
				skuID = sid
			}
		}
		if desc == "" && skuCode != "" {
			desc = skuCode
		}
		_, err = tx.Exec(ctx, `
INSERT INTO shipping_slip_lines (id, slip_id, sku_id, sku_code, description, quantity, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuid.New(), id, skuID, skuCode, desc, qty, i)
		if err != nil {
			return domain.ShippingSlip{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.ShippingSlip{}, err
	}
	return s.GetShippingSlip(ctx, req.OrgID, id.String())
}

func (s *Postgres) ListShippingSlips(ctx context.Context, filter domain.ShippingSlipFilter) ([]domain.ShippingSlip, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, filter.OrgRef)
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 40
	}

	q := `
SELECT s.id::text, s.org_id::text, s.slip_number,
       fb.code, tb.code,
       COALESCE(fw.code, ''), COALESCE(tw.code, ''),
       s.container_type, s.description, s.contents_summary, s.quantity_units, s.status,
       s.printed_at, s.shipped_at, s.received_at,
       s.created_by, s.operator_label, COALESCE(s.session_id::text, ''),
       s.notes, s.idempotency_key, s.created_at, s.updated_at
FROM shipping_slips s
JOIN branches fb ON fb.id = s.from_branch_id
JOIN branches tb ON tb.id = s.to_branch_id
LEFT JOIN warehouses fw ON fw.id = s.from_warehouse_id
LEFT JOIN warehouses tw ON tw.id = s.to_warehouse_id
WHERE 1=1`
	args := []any{}
	n := 1
	if filter.FromBranch != "" {
		q += fmt.Sprintf(` AND fb.code = $%d`, n)
		args = append(args, filter.FromBranch)
		n++
	}
	if filter.ToBranch != "" {
		q += fmt.Sprintf(` AND tb.code = $%d`, n)
		args = append(args, filter.ToBranch)
		n++
	}
	if filter.Status != "" {
		q += fmt.Sprintf(` AND s.status = $%d`, n)
		args = append(args, strings.ToUpper(filter.Status))
		n++
	}
	q += fmt.Sprintf(` ORDER BY s.created_at DESC LIMIT $%d`, n)
	args = append(args, limit)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.ShippingSlip
	for rows.Next() {
		slip, err := scanShippingSlip(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, slip)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Postgres) GetShippingSlip(ctx context.Context, orgRef, slipID string) (domain.ShippingSlip, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
SELECT s.id::text, s.org_id::text, s.slip_number,
       fb.code, tb.code,
       COALESCE(fw.code, ''), COALESCE(tw.code, ''),
       s.container_type, s.description, s.contents_summary, s.quantity_units, s.status,
       s.printed_at, s.shipped_at, s.received_at,
       s.created_by, s.operator_label, COALESCE(s.session_id::text, ''),
       s.notes, s.idempotency_key, s.created_at, s.updated_at
FROM shipping_slips s
JOIN branches fb ON fb.id = s.from_branch_id
JOIN branches tb ON tb.id = s.to_branch_id
LEFT JOIN warehouses fw ON fw.id = s.from_warehouse_id
LEFT JOIN warehouses tw ON tw.id = s.to_warehouse_id
WHERE s.id = $1::uuid`, slipID)
	slip, err := scanShippingSlip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ShippingSlip{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ShippingSlip{}, err
	}

	lines, err := loadSlipLines(ctx, tx, slip.ID)
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	slip.Lines = lines
	if err := tx.Commit(ctx); err != nil {
		return domain.ShippingSlip{}, err
	}
	return slip, nil
}

func (s *Postgres) MarkShippingSlipPrinted(ctx context.Context, orgRef, slipID, actor string) (domain.ShippingSlip, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM shipping_slips WHERE id = $1::uuid FOR UPDATE`, slipID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ShippingSlip{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	if status == domain.SlipStatusCancelled {
		return domain.ShippingSlip{}, errors.New("slip cancelled")
	}
	now := time.Now().UTC()
	next := domain.SlipStatusPrinted
	if status == domain.SlipStatusInTransit || status == domain.SlipStatusReceived {
		next = status
	}
	_, err = tx.Exec(ctx, `
UPDATE shipping_slips
SET status = $1, printed_at = COALESCE(printed_at, $2), updated_at = $2
WHERE id = $3::uuid`, next, now, slipID)
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	_ = actor
	if err := tx.Commit(ctx); err != nil {
		return domain.ShippingSlip{}, err
	}
	return s.GetShippingSlip(ctx, orgRef, slipID)
}

type slipScanner interface {
	Scan(dest ...any) error
}

func scanShippingSlip(row slipScanner) (domain.ShippingSlip, error) {
	var s domain.ShippingSlip
	var printed, shipped, received *time.Time
	err := row.Scan(
		&s.ID, &s.OrgID, &s.SlipNumber,
		&s.FromBranchID, &s.ToBranchID,
		&s.FromWarehouseID, &s.ToWarehouseID,
		&s.ContainerType, &s.Description, &s.ContentsSummary, &s.QuantityUnits, &s.Status,
		&printed, &shipped, &received,
		&s.CreatedBy, &s.OperatorLabel, &s.SessionID,
		&s.Notes, &s.IdempotencyKey, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	s.PrintedAt = printed
	s.ShippedAt = shipped
	s.ReceivedAt = received
	s.ContainerLabel = domain.ContainerTypeLabelES(s.ContainerType)
	return s, nil
}

func loadSlipLines(ctx context.Context, tx pgx.Tx, slipID string) ([]domain.ShippingSlipLine, error) {
	rows, err := tx.Query(ctx, `
SELECT id::text, COALESCE(sku_code, ''), description, quantity::float8, sort_order
FROM shipping_slip_lines WHERE slip_id = $1::uuid ORDER BY sort_order`, slipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ShippingSlipLine
	for rows.Next() {
		var l domain.ShippingSlipLine
		if err := rows.Scan(&l.ID, &l.SKU, &l.Description, &l.Quantity, &l.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
