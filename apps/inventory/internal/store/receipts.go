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

func (s *Postgres) ListWarehouses(ctx context.Context, filter domain.WarehouseFilter) ([]domain.Warehouse, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, filter.OrgRef)
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	q := `
SELECT w.code, b.code, w.name, COALESCE(w.warehouse_kind, 'STORE')
FROM warehouses w
JOIN branches b ON b.id = w.branch_id
WHERE 1=1`
	args := []any{}
	n := 1
	if filter.BranchCode != "" {
		q += fmt.Sprintf(` AND b.code = $%d`, n)
		args = append(args, filter.BranchCode)
		n++
	}
	if filter.Kind != "" {
		q += fmt.Sprintf(` AND COALESCE(w.warehouse_kind, 'STORE') = $%d`, n)
		args = append(args, strings.ToUpper(filter.Kind))
		n++
	}
	q += ` ORDER BY CASE COALESCE(w.warehouse_kind,'STORE') WHEN 'CEDI' THEN 0 WHEN 'ARRIVAL' THEN 1 ELSE 2 END, w.name`

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Warehouse
	for rows.Next() {
		var w domain.Warehouse
		if err := rows.Scan(&w.ID, &w.BranchID, &w.Name, &w.Kind); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Postgres) CreateReceipt(ctx context.Context, req domain.CreateReceiptRequest) (domain.Receipt, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return domain.Receipt{}, fmt.Errorf("idempotency_key required")
	}
	if len(req.Lines) == 0 {
		return domain.Receipt{}, fmt.Errorf("lines required")
	}
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, req.OrgID)
	})
	if err != nil {
		return domain.Receipt{}, err
	}
	defer tx.Rollback(ctx)

	var existingID string
	err = tx.QueryRow(ctx, `
SELECT id::text FROM inbound_receipts WHERE org_id = $1::uuid AND idempotency_key = $2`,
		orgID, req.IdempotencyKey).Scan(&existingID)
	if err == nil {
		_ = tx.Commit(ctx)
		return s.GetReceipt(ctx, req.OrgID, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Receipt{}, err
	}

	branchID, err := resolveBranchID(ctx, tx, orgID, req.BranchID)
	if err != nil {
		return domain.Receipt{}, err
	}
	warehouseID, err := resolveWarehouseID(ctx, tx, branchID, req.WarehouseID)
	if err != nil {
		return domain.Receipt{}, err
	}
	var whKind string
	_ = tx.QueryRow(ctx, `SELECT COALESCE(warehouse_kind,'STORE') FROM warehouses WHERE id = $1::uuid`, warehouseID).Scan(&whKind)

	printLabels := true
	if req.PrintLabels != nil {
		printLabels = *req.PrintLabels
	}
	var invoiceDate any
	if strings.TrimSpace(req.InvoiceDate) != "" {
		d, err := time.Parse("2006-01-02", strings.TrimSpace(req.InvoiceDate))
		if err != nil {
			return domain.Receipt{}, fmt.Errorf("invalid invoice_date")
		}
		invoiceDate = d
	}

	id := uuid.New()
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
INSERT INTO inbound_receipts (
  id, org_id, branch_id, warehouse_id, supplier_name, invoice_number, invoice_date,
  notes, status, print_labels, created_by, idempotency_key, created_at
) VALUES (
  $1, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7,
  $8, 'DRAFT', $9, $10, $11, $12
)`, id, orgID, branchID, warehouseID,
		strings.TrimSpace(req.SupplierName), strings.TrimSpace(req.InvoiceNumber), invoiceDate,
		strings.TrimSpace(req.Notes), printLabels, req.CreatedBy, req.IdempotencyKey, now)
	if err != nil {
		return domain.Receipt{}, err
	}

	for i, line := range req.Lines {
		if strings.TrimSpace(line.SKU) == "" || line.Quantity <= 0 {
			return domain.Receipt{}, fmt.Errorf("invalid line %d", i+1)
		}
		skuID, err := resolveSKUID(ctx, tx, line.SKU)
		if err != nil {
			return domain.Receipt{}, err
		}
		lineID := uuid.New()
		_, err = tx.Exec(ctx, `
INSERT INTO inbound_receipt_lines (
  id, receipt_id, sku_id, quantity, unit_cost, label_description, label_price, sort_order
) VALUES ($1, $2, $3::uuid, $4, $5, $6, $7, $8)`,
			lineID, id, skuID, line.Quantity, line.UnitCost,
			strings.TrimSpace(line.LabelDescription), line.LabelPrice, i)
		if err != nil {
			return domain.Receipt{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Receipt{}, err
	}
	rec, err := s.GetReceipt(ctx, req.OrgID, id.String())
	if err != nil {
		return domain.Receipt{}, err
	}
	rec.WarehouseKind = whKind
	return rec, nil
}

func (s *Postgres) ListReceipts(ctx context.Context, filter domain.ReceiptFilter) ([]domain.Receipt, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
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
SELECT r.id::text, o.id::text, b.code, w.code, COALESCE(w.warehouse_kind,'STORE'),
       r.supplier_name, r.invoice_number,
       CASE WHEN r.invoice_date IS NULL THEN NULL ELSE to_char(r.invoice_date, 'YYYY-MM-DD') END,
       r.notes, r.status, r.print_labels, r.posted_at, COALESCE(r.posted_by::text,''),
       r.created_by, r.idempotency_key, r.created_at
FROM inbound_receipts r
JOIN organizations o ON o.id = r.org_id
JOIN branches b ON b.id = r.branch_id
JOIN warehouses w ON w.id = r.warehouse_id
WHERE r.org_id = $1::uuid`
	args := []any{orgID}
	n := 2
	if filter.BranchCode != "" {
		q += fmt.Sprintf(` AND b.code = $%d`, n)
		args = append(args, filter.BranchCode)
		n++
	}
	if filter.WarehouseID != "" {
		q += fmt.Sprintf(` AND w.code = $%d`, n)
		args = append(args, filter.WarehouseID)
		n++
	}
	if filter.Status != "" {
		q += fmt.Sprintf(` AND r.status = $%d`, n)
		args = append(args, strings.ToUpper(filter.Status))
		n++
	}
	q += fmt.Sprintf(` ORDER BY r.created_at DESC LIMIT $%d`, n)
	args = append(args, limit)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Receipt
	for rows.Next() {
		rec, err := scanReceiptHeader(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Postgres) GetReceipt(ctx context.Context, orgRef, receiptID string) (domain.Receipt, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.Receipt{}, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
SELECT r.id::text, o.id::text, b.code, w.code, COALESCE(w.warehouse_kind,'STORE'),
       r.supplier_name, r.invoice_number,
       CASE WHEN r.invoice_date IS NULL THEN NULL ELSE to_char(r.invoice_date, 'YYYY-MM-DD') END,
       r.notes, r.status, r.print_labels, r.posted_at, COALESCE(r.posted_by::text,''),
       r.created_by, r.idempotency_key, r.created_at
FROM inbound_receipts r
JOIN organizations o ON o.id = r.org_id
JOIN branches b ON b.id = r.branch_id
JOIN warehouses w ON w.id = r.warehouse_id
WHERE r.org_id = $1::uuid AND r.id = $2::uuid`, orgID, receiptID)
	rec, err := scanReceiptHeader(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Receipt{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Receipt{}, err
	}

	lines, err := loadReceiptLines(ctx, tx, receiptID)
	if err != nil {
		return domain.Receipt{}, err
	}
	rec.Lines = lines

	if rec.Status == domain.ReceiptStatusPosted && rec.PrintLabels {
		labels, err := labelsForReceipt(ctx, tx, orgID, rec.BranchID, lines)
		if err == nil {
			rec.Labels = labels
		}
	}
	_ = tx.Commit(ctx)
	return rec, nil
}

func (s *Postgres) PostReceipt(ctx context.Context, orgRef, receiptID, postedBy string) (domain.Receipt, error) {
	// Digimon: Digivolve — a DRAFT receipt evolves into a POSTED stock fact.
	return s.DigivolveReceipt(ctx, orgRef, receiptID, postedBy)
}

// DigivolveReceipt posts a DRAFT inbound receipt (stock + optional labels).
// Justification: Digimon digivolve from weaker to stronger forms; a draft receipt
// digivolves into posted RECEIPT movements and reception labels.
func (s *Postgres) DigivolveReceipt(ctx context.Context, orgRef, receiptID, postedBy string) (domain.Receipt, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.Receipt{}, err
	}
	defer tx.Rollback(ctx)

	var status, branchUUID, warehouseUUID, branchCode, warehouseCode string
	var printLabels bool
	err = tx.QueryRow(ctx, `
SELECT r.status, r.branch_id::text, r.warehouse_id::text, b.code, w.code, r.print_labels
FROM inbound_receipts r
JOIN branches b ON b.id = r.branch_id
JOIN warehouses w ON w.id = r.warehouse_id
WHERE r.org_id = $1::uuid AND r.id = $2::uuid
FOR UPDATE OF r`, orgID, receiptID).Scan(&status, &branchUUID, &warehouseUUID, &branchCode, &warehouseCode, &printLabels)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Receipt{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Receipt{}, err
	}
	if status == domain.ReceiptStatusPosted {
		_ = tx.Commit(ctx)
		return s.GetReceipt(ctx, orgRef, receiptID)
	}
	if status != domain.ReceiptStatusDraft {
		return domain.Receipt{}, fmt.Errorf("receipt not draft")
	}

	lines, err := loadReceiptLines(ctx, tx, receiptID)
	if err != nil {
		return domain.Receipt{}, err
	}
	if len(lines) == 0 {
		return domain.Receipt{}, fmt.Errorf("receipt has no lines")
	}

	userID, err := resolveUserID(ctx, tx, orgID, postedBy)
	if err != nil {
		return domain.Receipt{}, err
	}

	for _, line := range lines {
		skuID, err := resolveSKUID(ctx, tx, line.SKU)
		if err != nil {
			return domain.Receipt{}, err
		}
		// Ensure balance exists then apply RECEIPT delta inside this transaction.
		movID, err := applyReceiptLine(ctx, tx, orgID, branchUUID, warehouseUUID, skuID, line.Quantity, postedBy, receiptID, line.ID)
		if err != nil {
			return domain.Receipt{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE inbound_receipt_lines SET movement_id = $1::uuid WHERE id = $2::uuid`, movID, line.ID)
		if err != nil {
			return domain.Receipt{}, err
		}

		if printLabels {
			if err := upsertReceptionLabel(ctx, tx, orgID, branchUUID, skuID, line); err != nil {
				return domain.Receipt{}, err
			}
		}
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
UPDATE inbound_receipts
SET status = 'POSTED', posted_at = $1, posted_by = $2
WHERE id = $3::uuid`, now, userID, receiptID)
	if err != nil {
		return domain.Receipt{}, err
	}

	_, _ = tx.Exec(ctx, `
INSERT INTO outbox (event_type, payload) VALUES ('InboundReceiptPosted', jsonb_build_object(
  'receipt_id', $1::text,
  'org_id', $2::text,
  'branch_id', $3::text,
  'warehouse_id', $4::text,
  'lines', $5::int
))`, receiptID, orgID, branchCode, warehouseCode, len(lines))

	if err := tx.Commit(ctx); err != nil {
		return domain.Receipt{}, err
	}
	return s.GetReceipt(ctx, orgRef, receiptID)
}

func applyReceiptLine(
	ctx context.Context, tx pgx.Tx,
	orgID, branchUUID, warehouseUUID, skuID string,
	qty float64, postedBy, receiptID, lineID string,
) (string, error) {
	if qty <= 0 {
		return "", fmt.Errorf("quantity must be positive")
	}
	var balID string
	var onHand float64
	var version int
	err := tx.QueryRow(ctx, `
SELECT id::text, on_hand::float8, version
FROM stock_balances
WHERE warehouse_id = $1::uuid AND sku_id = $2::uuid
FOR UPDATE`, warehouseUUID, skuID).Scan(&balID, &onHand, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		balID = uuid.New().String()
		_, err = tx.Exec(ctx, `
INSERT INTO stock_balances (id, warehouse_id, sku_id, on_hand, reserved, version, updated_at)
VALUES ($1::uuid, $2::uuid, $3::uuid, 0, 0, 1, now())`, balID, warehouseUUID, skuID)
		if err != nil {
			return "", err
		}
		onHand = 0
		version = 1
	} else if err != nil {
		return "", err
	}

	next := onHand + qty
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

	userID, err := resolveUserID(ctx, tx, orgID, postedBy)
	if err != nil {
		return "", err
	}
	movID := uuid.New()
	idem := fmt.Sprintf("receipt:%s:line:%s", receiptID, lineID)
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
INSERT INTO inventory_movements (
  id, org_id, branch_id, sku_id, warehouse_id, movement_type, quantity, status,
  posted_by, idempotency_key, created_at, source_receipt_id
) VALUES (
  $1, $2::uuid, $3::uuid, $4::uuid, $5::uuid, 'RECEIPT', $6, 'POSTED',
  $7, $8, $9, $10::uuid
)`, movID, orgID, branchUUID, skuID, warehouseUUID, qty, userID, idem, now, receiptID)
	if err != nil {
		return "", err
	}
	return movID.String(), nil
}

func upsertReceptionLabel(ctx context.Context, tx pgx.Tx, orgID, branchUUID, skuID string, line domain.ReceiptLine) error {
	desc := strings.TrimSpace(line.LabelDescription)
	if desc == "" {
		_ = tx.QueryRow(ctx, `
SELECT COALESCE(NULLIF(l.public_description,''), p.name, ps.sku)
FROM product_skus ps
JOIN products p ON p.id = ps.product_id
LEFT JOIN store_sku_labels l ON l.sku_id = ps.id AND l.branch_id = $2::uuid
WHERE ps.id = $1::uuid`, skuID, branchUUID).Scan(&desc)
	}
	if desc == "" {
		desc = line.SKU
	}
	var price any
	if line.LabelPrice != nil {
		price = *line.LabelPrice
	}
	_, err := tx.Exec(ctx, `
INSERT INTO store_sku_labels (
  org_id, branch_id, sku_id, public_description, currency, common_price, price_mode, active, updated_at
) VALUES (
  $1::uuid, $2::uuid, $3::uuid, $4, 'MXN', $5, 'COMMON', true, now()
)
ON CONFLICT (branch_id, sku_id) DO UPDATE SET
  public_description = CASE
    WHEN EXCLUDED.public_description <> '' THEN EXCLUDED.public_description
    ELSE store_sku_labels.public_description
  END,
  common_price = COALESCE(EXCLUDED.common_price, store_sku_labels.common_price),
  updated_at = now(),
  active = true`, orgID, branchUUID, skuID, desc, price)
	return err
}

func loadReceiptLines(ctx context.Context, tx pgx.Tx, receiptID string) ([]domain.ReceiptLine, error) {
	rows, err := tx.Query(ctx, `
SELECT l.id::text, ps.sku, l.quantity::float8, l.unit_cost, l.label_description, l.label_price,
       COALESCE(l.movement_id::text, ''), l.sort_order
FROM inbound_receipt_lines l
JOIN product_skus ps ON ps.id = l.sku_id
WHERE l.receipt_id = $1::uuid
ORDER BY l.sort_order, l.id`, receiptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ReceiptLine
	for rows.Next() {
		var line domain.ReceiptLine
		var unitCost, labelPrice *float64
		if err := rows.Scan(&line.ID, &line.SKU, &line.Quantity, &unitCost, &line.LabelDescription, &labelPrice, &line.MovementID, &line.SortOrder); err != nil {
			return nil, err
		}
		line.UnitCost = unitCost
		line.LabelPrice = labelPrice
		out = append(out, line)
	}
	return out, rows.Err()
}

func labelsForReceipt(ctx context.Context, tx pgx.Tx, orgID, branchCode string, lines []domain.ReceiptLine) ([]domain.StoreLabel, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	skus := make([]string, 0, len(lines))
	for _, l := range lines {
		skus = append(skus, l.SKU)
	}
	rows, err := tx.Query(ctx, `
SELECT l.id::text, b.code, COALESCE(l.store_display_name, b.name, ''),
       ps.sku, COALESCE(ps.material_code,''), COALESCE(ps.barcode,''),
       COALESCE(ps.size_code,''), COALESCE(ps.color_code,''),
       COALESCE(NULLIF(l.brand_label,''), ps.brand, ''),
       l.public_description, COALESCE(l.department_label,''),
       l.currency, l.common_price, l.special_price, l.final_price, l.price_mode
FROM store_sku_labels l
JOIN branches b ON b.id = l.branch_id
JOIN product_skus ps ON ps.id = l.sku_id
WHERE l.org_id = $1::uuid AND b.code = $2 AND ps.sku = ANY($3)
  AND l.active`, orgID, branchCode, skus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StoreLabel
	for rows.Next() {
		var lbl domain.StoreLabel
		var common, special, final *float64
		if err := rows.Scan(
			&lbl.ID, &lbl.BranchID, &lbl.StoreDisplayName,
			&lbl.SKU, &lbl.MaterialCode, &lbl.Barcode,
			&lbl.SizeCode, &lbl.ColorCode, &lbl.Brand,
			&lbl.PublicDescription, &lbl.DepartmentLabel,
			&lbl.Currency, &common, &special, &final, &lbl.PriceMode,
		); err != nil {
			return nil, err
		}
		lbl.CommonPrice = common
		lbl.SpecialPrice = special
		lbl.FinalPrice = final
		lbl.EffectivePrice = lbl.ResolveEffectivePrice()
		out = append(out, lbl)
	}
	return out, rows.Err()
}

type receiptScanner interface {
	Scan(dest ...any) error
}

func scanReceiptHeader(row receiptScanner) (domain.Receipt, error) {
	var rec domain.Receipt
	var invoiceDate *string
	var postedAt *time.Time
	var postedBy string
	err := row.Scan(
		&rec.ID, &rec.OrgID, &rec.BranchID, &rec.WarehouseID, &rec.WarehouseKind,
		&rec.SupplierName, &rec.InvoiceNumber, &invoiceDate,
		&rec.Notes, &rec.Status, &rec.PrintLabels, &postedAt, &postedBy,
		&rec.CreatedBy, &rec.IdempotencyKey, &rec.CreatedAt,
	)
	if err != nil {
		return domain.Receipt{}, err
	}
	rec.InvoiceDate = invoiceDate
	rec.PostedAt = postedAt
	rec.PostedBy = postedBy
	return rec, nil
}
