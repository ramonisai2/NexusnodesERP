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

func (s *Postgres) CreateTransfer(ctx context.Context, req domain.CreateTransferRequest) (domain.InventoryTransfer, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return domain.InventoryTransfer{}, errors.New("idempotency_key required")
	}
	if strings.TrimSpace(req.FromBranchID) == "" || strings.TrimSpace(req.ToBranchID) == "" {
		return domain.InventoryTransfer{}, errors.New("from_branch_id and to_branch_id required")
	}
	if strings.TrimSpace(req.FromWarehouseID) == "" || strings.TrimSpace(req.ToWarehouseID) == "" {
		return domain.InventoryTransfer{}, errors.New("from_warehouse_id and to_warehouse_id required")
	}
	if len(req.Lines) == 0 {
		return domain.InventoryTransfer{}, errors.New("lines required")
	}
	for i, line := range req.Lines {
		if strings.TrimSpace(line.SKU) == "" {
			return domain.InventoryTransfer{}, fmt.Errorf("line %d: sku required", i)
		}
		if line.Quantity <= 0 {
			return domain.InventoryTransfer{}, fmt.Errorf("line %d: quantity must be positive", i)
		}
	}

	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, req.OrgID)
	})
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	defer tx.Rollback(ctx)

	var existingID string
	err = tx.QueryRow(ctx, `
SELECT id::text FROM inventory_transfers WHERE org_id = $1 AND idempotency_key = $2`,
		orgID, req.IdempotencyKey).Scan(&existingID)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return domain.InventoryTransfer{}, err
		}
		return s.GetTransfer(ctx, req.OrgID, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.InventoryTransfer{}, err
	}

	fromBranch, err := resolveBranchID(ctx, tx, orgID, req.FromBranchID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	toBranch, err := resolveBranchID(ctx, tx, orgID, req.ToBranchID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	fromWH, err := resolveWarehouseID(ctx, tx, fromBranch, req.FromWarehouseID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	toWH, err := resolveWarehouseID(ctx, tx, toBranch, req.ToWarehouseID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	if fromBranch == toBranch && fromWH == toWH {
		return domain.InventoryTransfer{}, errors.New("source and destination warehouse must differ")
	}

	var sess any
	if req.SessionID != "" {
		if _, err := uuid.Parse(req.SessionID); err == nil {
			sess = req.SessionID
		}
	}

	id := uuid.New()
	now := time.Now().UTC()
	number := fmt.Sprintf("TRF-%s", strings.ToUpper(id.String()[:8]))

	_, err = tx.Exec(ctx, `
INSERT INTO inventory_transfers (
  id, org_id, transfer_number, from_branch_id, to_branch_id, from_warehouse_id, to_warehouse_id,
  status, notes, created_by, operator_label, session_id, idempotency_key, created_at, updated_at
) VALUES (
  $1, $2::uuid, $3, $4::uuid, $5::uuid, $6::uuid, $7::uuid,
  'DRAFT', $8, $9, $10, $11, $12, $13, $13
)`, id, orgID, number, fromBranch, toBranch, fromWH, toWH,
		strings.TrimSpace(req.Notes), req.CreatedBy, strings.TrimSpace(req.OperatorLabel),
		sess, req.IdempotencyKey, now)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}

	for i, line := range req.Lines {
		skuID, err := resolveSKUID(ctx, tx, line.SKU)
		if err != nil {
			return domain.InventoryTransfer{}, fmt.Errorf("sku %s: %w", line.SKU, err)
		}
		_, err = tx.Exec(ctx, `
INSERT INTO inventory_transfer_lines (id, transfer_id, sku_id, quantity, sort_order)
VALUES ($1, $2, $3::uuid, $4, $5)`,
			uuid.New(), id, skuID, line.Quantity, i)
		if err != nil {
			return domain.InventoryTransfer{}, err
		}
	}

	for _, slipID := range uniqueStrings(req.SlipIDs) {
		var ok bool
		err = tx.QueryRow(ctx, `
SELECT true FROM shipping_slips WHERE org_id = $1::uuid AND id = $2::uuid`, orgID, slipID).Scan(&ok)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.InventoryTransfer{}, fmt.Errorf("slip not found: %s", slipID)
		}
		if err != nil {
			return domain.InventoryTransfer{}, err
		}
		_, err = tx.Exec(ctx, `
INSERT INTO inventory_transfer_slip_links (transfer_id, shipping_slip_id)
VALUES ($1, $2::uuid) ON CONFLICT DO NOTHING`, id, slipID)
		if err != nil {
			return domain.InventoryTransfer{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.InventoryTransfer{}, err
	}
	return s.GetTransfer(ctx, req.OrgID, id.String())
}

func (s *Postgres) ListTransfers(ctx context.Context, filter domain.TransferFilter) ([]domain.InventoryTransfer, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	var out []domain.InventoryTransfer
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}
		q := `
SELECT t.id::text, t.org_id::text, t.transfer_number,
       fb.code, tb.code, fw.code, tw.code,
       t.status, t.notes, t.created_by, t.operator_label,
       t.shipped_by, t.shipped_at, t.received_by, t.received_at,
       t.cancelled_by, t.cancelled_at, t.cancel_reason,
       t.idempotency_key, t.created_at
FROM inventory_transfers t
JOIN branches fb ON fb.id = t.from_branch_id
JOIN branches tb ON tb.id = t.to_branch_id
JOIN warehouses fw ON fw.id = t.from_warehouse_id
JOIN warehouses tw ON tw.id = t.to_warehouse_id
WHERE t.org_id = $1::uuid`
		args := []any{orgID}
		n := 2
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
			q += fmt.Sprintf(` AND t.status = $%d`, n)
			args = append(args, strings.ToUpper(filter.Status))
			n++
		}
		q += fmt.Sprintf(` ORDER BY t.created_at DESC LIMIT $%d`, n)
		args = append(args, limit)

		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t domain.InventoryTransfer
			if err := rows.Scan(
				&t.ID, &t.OrgID, &t.TransferNumber,
				&t.FromBranchID, &t.ToBranchID, &t.FromWarehouseID, &t.ToWarehouseID,
				&t.Status, &t.Notes, &t.CreatedBy, &t.OperatorLabel,
				&t.ShippedBy, &t.ShippedAt, &t.ReceivedBy, &t.ReceivedAt,
				&t.CancelledBy, &t.CancelledAt, &t.CancelReason,
				&t.IdempotencyKey, &t.CreatedAt,
			); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Postgres) GetTransfer(ctx context.Context, orgRef, transferID string) (domain.InventoryTransfer, error) {
	var out domain.InventoryTransfer
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
SELECT t.id::text, t.org_id::text, t.transfer_number,
       fb.code, tb.code, fw.code, tw.code,
       t.status, t.notes, t.created_by, t.operator_label,
       t.shipped_by, t.shipped_at, t.received_by, t.received_at,
       t.cancelled_by, t.cancelled_at, t.cancel_reason,
       t.idempotency_key, t.created_at
FROM inventory_transfers t
JOIN branches fb ON fb.id = t.from_branch_id
JOIN branches tb ON tb.id = t.to_branch_id
JOIN warehouses fw ON fw.id = t.from_warehouse_id
JOIN warehouses tw ON tw.id = t.to_warehouse_id
WHERE t.org_id = $1::uuid AND t.id = $2::uuid`, orgID, transferID).Scan(
			&out.ID, &out.OrgID, &out.TransferNumber,
			&out.FromBranchID, &out.ToBranchID, &out.FromWarehouseID, &out.ToWarehouseID,
			&out.Status, &out.Notes, &out.CreatedBy, &out.OperatorLabel,
			&out.ShippedBy, &out.ShippedAt, &out.ReceivedBy, &out.ReceivedAt,
			&out.CancelledBy, &out.CancelledAt, &out.CancelReason,
			&out.IdempotencyKey, &out.CreatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		lines, err := loadTransferLines(ctx, tx, transferID)
		if err != nil {
			return err
		}
		out.Lines = lines
		slips, err := loadTransferSlipIDs(ctx, tx, transferID)
		if err != nil {
			return err
		}
		out.SlipIDs = slips
		return nil
	})
	return out, err
}

func (s *Postgres) ShipTransfer(ctx context.Context, orgRef, transferID, actor string) (domain.InventoryTransfer, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	defer tx.Rollback(ctx)

	hdr, err := lockTransferHeader(ctx, tx, orgID, transferID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	if hdr.Status == domain.TransferStatusInTransit || hdr.Status == domain.TransferStatusReceived {
		_ = tx.Commit(ctx)
		return s.GetTransfer(ctx, orgRef, transferID)
	}
	if hdr.Status != domain.TransferStatusDraft {
		return domain.InventoryTransfer{}, fmt.Errorf("transfer not draft")
	}

	lines, err := loadTransferLinesRaw(ctx, tx, transferID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	if len(lines) == 0 {
		return domain.InventoryTransfer{}, errors.New("transfer has no lines")
	}

	userID, err := resolveUserID(ctx, tx, orgID, actor)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}

	for _, line := range lines {
		movID, err := applyTransferStock(ctx, tx, transferStockInput{
			OrgID:          orgID,
			BranchUUID:     hdr.FromBranchUUID,
			WarehouseUUID:  hdr.FromWarehouseUUID,
			SkuUUID:        line.SkuUUID,
			Qty:            line.Quantity,
			MovementType:   "TRANSFER_OUT",
			PostedBy:       userID,
			TransferID:     transferID,
			LineID:         line.ID,
			IdempotencyKey: fmt.Sprintf("transfer:%s:ship:line:%s", transferID, line.ID),
		})
		if err != nil {
			return domain.InventoryTransfer{}, err
		}
		_, err = tx.Exec(ctx, `
UPDATE inventory_transfer_lines SET out_movement_id = $1::uuid WHERE id = $2::uuid`, movID, line.ID)
		if err != nil {
			return domain.InventoryTransfer{}, err
		}
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
UPDATE inventory_transfers
SET status = 'IN_TRANSIT', shipped_at = $1, shipped_by = $2, updated_at = $1
WHERE id = $3::uuid`, now, actor, transferID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}

	// Advance linked papeletas to IN_TRANSIT when still DRAFT/PRINTED.
	_, _ = tx.Exec(ctx, `
UPDATE shipping_slips s
SET status = 'IN_TRANSIT', shipped_at = COALESCE(s.shipped_at, $1), updated_at = $1
FROM inventory_transfer_slip_links l
WHERE l.transfer_id = $2::uuid AND l.shipping_slip_id = s.id
  AND s.status IN ('DRAFT', 'PRINTED')`, now, transferID)

	_, _ = tx.Exec(ctx, `
INSERT INTO outbox (event_type, payload) VALUES ('InventoryTransferShipped', jsonb_build_object(
  'transfer_id', $1::text,
  'org_id', $2::text,
  'from_branch', $3::text,
  'to_branch', $4::text,
  'lines', $5::int
))`, transferID, orgID, hdr.FromBranchCode, hdr.ToBranchCode, len(lines))

	if err := tx.Commit(ctx); err != nil {
		return domain.InventoryTransfer{}, err
	}
	return s.GetTransfer(ctx, orgRef, transferID)
}

func (s *Postgres) ReceiveTransfer(ctx context.Context, orgRef, transferID, actor string) (domain.InventoryTransfer, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	defer tx.Rollback(ctx)

	hdr, err := lockTransferHeader(ctx, tx, orgID, transferID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	if hdr.Status == domain.TransferStatusReceived {
		_ = tx.Commit(ctx)
		return s.GetTransfer(ctx, orgRef, transferID)
	}
	if hdr.Status != domain.TransferStatusInTransit {
		return domain.InventoryTransfer{}, fmt.Errorf("transfer not in transit")
	}

	lines, err := loadTransferLinesRaw(ctx, tx, transferID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	userID, err := resolveUserID(ctx, tx, orgID, actor)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}

	for _, line := range lines {
		movID, err := applyTransferStock(ctx, tx, transferStockInput{
			OrgID:          orgID,
			BranchUUID:     hdr.ToBranchUUID,
			WarehouseUUID:  hdr.ToWarehouseUUID,
			SkuUUID:        line.SkuUUID,
			Qty:            line.Quantity,
			MovementType:   "TRANSFER_IN",
			PostedBy:       userID,
			TransferID:     transferID,
			LineID:         line.ID,
			IdempotencyKey: fmt.Sprintf("transfer:%s:receive:line:%s", transferID, line.ID),
		})
		if err != nil {
			return domain.InventoryTransfer{}, err
		}
		_, err = tx.Exec(ctx, `
UPDATE inventory_transfer_lines SET in_movement_id = $1::uuid WHERE id = $2::uuid`, movID, line.ID)
		if err != nil {
			return domain.InventoryTransfer{}, err
		}
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
UPDATE inventory_transfers
SET status = 'RECEIVED', received_at = $1, received_by = $2, updated_at = $1
WHERE id = $3::uuid`, now, actor, transferID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}

	_, _ = tx.Exec(ctx, `
UPDATE shipping_slips s
SET status = 'RECEIVED', received_at = COALESCE(s.received_at, $1), updated_at = $1
FROM inventory_transfer_slip_links l
WHERE l.transfer_id = $2::uuid AND l.shipping_slip_id = s.id
  AND s.status IN ('DRAFT', 'PRINTED', 'IN_TRANSIT')`, now, transferID)

	_, _ = tx.Exec(ctx, `
INSERT INTO outbox (event_type, payload) VALUES ('InventoryTransferReceived', jsonb_build_object(
  'transfer_id', $1::text,
  'org_id', $2::text,
  'from_branch', $3::text,
  'to_branch', $4::text,
  'lines', $5::int
))`, transferID, orgID, hdr.FromBranchCode, hdr.ToBranchCode, len(lines))

	if err := tx.Commit(ctx); err != nil {
		return domain.InventoryTransfer{}, err
	}
	return s.GetTransfer(ctx, orgRef, transferID)
}

func (s *Postgres) CancelTransfer(ctx context.Context, orgRef, transferID string, req domain.CancelTransferRequest) (domain.InventoryTransfer, error) {
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		return domain.InventoryTransfer{}, errors.New("actor required")
	}
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	defer tx.Rollback(ctx)

	hdr, err := lockTransferHeader(ctx, tx, orgID, transferID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}
	if hdr.Status == domain.TransferStatusCancelled {
		_ = tx.Commit(ctx)
		return s.GetTransfer(ctx, orgRef, transferID)
	}
	if hdr.Status == domain.TransferStatusReceived {
		return domain.InventoryTransfer{}, errors.New("cannot cancel received transfer")
	}

	now := time.Now().UTC()
	if hdr.Status == domain.TransferStatusInTransit {
		lines, err := loadTransferLinesRaw(ctx, tx, transferID)
		if err != nil {
			return domain.InventoryTransfer{}, err
		}
		userID, err := resolveUserID(ctx, tx, orgID, actor)
		if err != nil {
			return domain.InventoryTransfer{}, err
		}
		// Return stock to source warehouse.
		for _, line := range lines {
			_, err := applyTransferStock(ctx, tx, transferStockInput{
				OrgID:          orgID,
				BranchUUID:     hdr.FromBranchUUID,
				WarehouseUUID:  hdr.FromWarehouseUUID,
				SkuUUID:        line.SkuUUID,
				Qty:            line.Quantity,
				MovementType:   "TRANSFER_IN",
				PostedBy:       userID,
				TransferID:     transferID,
				LineID:         line.ID,
				IdempotencyKey: fmt.Sprintf("transfer:%s:cancel:line:%s", transferID, line.ID),
			})
			if err != nil {
				return domain.InventoryTransfer{}, err
			}
		}
	}

	_, err = tx.Exec(ctx, `
UPDATE inventory_transfers
SET status = 'CANCELLED', cancelled_at = $1, cancelled_by = $2, cancel_reason = $3, updated_at = $1
WHERE id = $4::uuid`, now, actor, strings.TrimSpace(req.Reason), transferID)
	if err != nil {
		return domain.InventoryTransfer{}, err
	}

	_, _ = tx.Exec(ctx, `
INSERT INTO outbox (event_type, payload) VALUES ('InventoryTransferCancelled', jsonb_build_object(
  'transfer_id', $1::text,
  'org_id', $2::text,
  'reason', $3::text
))`, transferID, orgID, strings.TrimSpace(req.Reason))

	if err := tx.Commit(ctx); err != nil {
		return domain.InventoryTransfer{}, err
	}
	return s.GetTransfer(ctx, orgRef, transferID)
}

type transferHeader struct {
	Status             string
	FromBranchUUID     string
	ToBranchUUID       string
	FromWarehouseUUID  string
	ToWarehouseUUID    string
	FromBranchCode     string
	ToBranchCode       string
	FromWarehouseCode  string
	ToWarehouseCode    string
}

type transferLineRaw struct {
	ID       string
	SkuUUID  string
	SKU      string
	Quantity float64
}

func lockTransferHeader(ctx context.Context, tx pgx.Tx, orgID, transferID string) (transferHeader, error) {
	var h transferHeader
	err := tx.QueryRow(ctx, `
SELECT t.status, t.from_branch_id::text, t.to_branch_id::text,
       t.from_warehouse_id::text, t.to_warehouse_id::text,
       fb.code, tb.code, fw.code, tw.code
FROM inventory_transfers t
JOIN branches fb ON fb.id = t.from_branch_id
JOIN branches tb ON tb.id = t.to_branch_id
JOIN warehouses fw ON fw.id = t.from_warehouse_id
JOIN warehouses tw ON tw.id = t.to_warehouse_id
WHERE t.org_id = $1::uuid AND t.id = $2::uuid
FOR UPDATE OF t`, orgID, transferID).Scan(
		&h.Status, &h.FromBranchUUID, &h.ToBranchUUID,
		&h.FromWarehouseUUID, &h.ToWarehouseUUID,
		&h.FromBranchCode, &h.ToBranchCode, &h.FromWarehouseCode, &h.ToWarehouseCode,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, domain.ErrNotFound
	}
	return h, err
}

func loadTransferLines(ctx context.Context, tx pgx.Tx, transferID string) ([]domain.TransferLine, error) {
	rows, err := tx.Query(ctx, `
SELECT l.id::text, ps.sku, l.quantity::float8, l.sort_order,
       COALESCE(l.out_movement_id::text, ''), COALESCE(l.in_movement_id::text, '')
FROM inventory_transfer_lines l
JOIN product_skus ps ON ps.id = l.sku_id
WHERE l.transfer_id = $1::uuid
ORDER BY l.sort_order, l.id`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TransferLine
	for rows.Next() {
		var line domain.TransferLine
		if err := rows.Scan(&line.ID, &line.SKU, &line.Quantity, &line.SortOrder, &line.OutMovementID, &line.InMovementID); err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

func loadTransferLinesRaw(ctx context.Context, tx pgx.Tx, transferID string) ([]transferLineRaw, error) {
	rows, err := tx.Query(ctx, `
SELECT l.id::text, l.sku_id::text, ps.sku, l.quantity::float8
FROM inventory_transfer_lines l
JOIN product_skus ps ON ps.id = l.sku_id
WHERE l.transfer_id = $1::uuid
ORDER BY l.sort_order, l.id`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []transferLineRaw
	for rows.Next() {
		var line transferLineRaw
		if err := rows.Scan(&line.ID, &line.SkuUUID, &line.SKU, &line.Quantity); err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

func loadTransferSlipIDs(ctx context.Context, tx pgx.Tx, transferID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
SELECT shipping_slip_id::text FROM inventory_transfer_slip_links WHERE transfer_id = $1::uuid`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

type transferStockInput struct {
	OrgID          string
	BranchUUID     string
	WarehouseUUID  string
	SkuUUID        string
	Qty            float64
	MovementType   string
	PostedBy       any // *string user uuid or nil
	TransferID     string
	LineID         string
	IdempotencyKey string
}

func applyTransferStock(ctx context.Context, tx pgx.Tx, in transferStockInput) (string, error) {
	if in.Qty <= 0 {
		return "", fmt.Errorf("quantity must be positive")
	}
	delta, err := CowabungaDelta(in.MovementType, in.Qty)
	if err != nil {
		return "", err
	}

	var balID string
	var onHand float64
	var version int
	err = tx.QueryRow(ctx, `
SELECT id::text, on_hand::float8, version
FROM stock_balances
WHERE warehouse_id = $1::uuid AND sku_id = $2::uuid
FOR UPDATE`, in.WarehouseUUID, in.SkuUUID).Scan(&balID, &onHand, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		if delta < 0 {
			return "", domain.ErrInsufficientStock
		}
		balID = uuid.New().String()
		_, err = tx.Exec(ctx, `
INSERT INTO stock_balances (id, warehouse_id, sku_id, on_hand, reserved, version, updated_at)
VALUES ($1::uuid, $2::uuid, $3::uuid, 0, 0, 1, now())`, balID, in.WarehouseUUID, in.SkuUUID)
		if err != nil {
			return "", err
		}
		onHand = 0
		version = 1
	} else if err != nil {
		return "", err
	}

	next := onHand + delta
	if next < 0 {
		return "", domain.ErrInsufficientStock
	}
	ct, err := tx.Exec(ctx, `
UPDATE stock_balances
SET on_hand = $1, version = version + 1, updated_at = now()
WHERE id = $2::uuid AND version = $3`, next, balID, version)
	if err != nil {
		return "", err
	}
	if ct.RowsAffected() == 0 {
		return "", domain.ErrConflict
	}

	movID := uuid.New()
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
INSERT INTO inventory_movements (
  id, org_id, branch_id, sku_id, warehouse_id, movement_type, quantity, status,
  posted_by, idempotency_key, created_at, source_transfer_id, transfer_line_id
) VALUES (
  $1, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, 'POSTED',
  $8, $9, $10, $11::uuid, $12::uuid
)`, movID, in.OrgID, in.BranchUUID, in.SkuUUID, in.WarehouseUUID, in.MovementType, in.Qty,
		in.PostedBy, in.IdempotencyKey, now, in.TransferID, in.LineID)
	if err != nil {
		return "", err
	}

	_, _ = tx.Exec(ctx, `
INSERT INTO outbox (event_type, payload) VALUES ('InventoryMoved', jsonb_build_object(
  'movement_id', $1::text,
  'movement_type', $2::text,
  'quantity', $3::float8,
  'transfer_id', $4::text
))`, movID.String(), in.MovementType, in.Qty, in.TransferID)

	return movID.String(), nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
