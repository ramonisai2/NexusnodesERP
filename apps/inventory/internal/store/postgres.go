package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func (s *Postgres) ListBalances(ctx context.Context, filter domain.BalanceFilter) ([]domain.StockBalance, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, filter.OrgRef)
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	q := `
SELECT sb.id::text, w.code, b.code, ps.sku, ps.sku, p.name,
       sb.on_hand::float8, sb.reserved::float8, sb.version,
       COALESCE((
         SELECT array_agg(DISTINCT sd.code ORDER BY sd.code)
         FROM product_placements pp
         JOIN store_departments sd ON sd.id = pp.department_id
         WHERE pp.product_id = p.id AND pp.active AND sd.branch_id = b.id
       ), '{}') AS dept_codes,
       COALESCE((
         SELECT array_agg(DISTINCT dc.code ORDER BY dc.code)
         FROM product_placements pp
         JOIN department_categories dc ON dc.id = pp.category_id
         JOIN store_departments sd ON sd.id = pp.department_id
         WHERE pp.product_id = p.id AND pp.active AND sd.branch_id = b.id AND pp.category_id IS NOT NULL
       ), '{}') AS cat_codes,
       COALESCE((
         SELECT array_agg(DISTINCT (sd.name || ' / ' || COALESCE(dc.name, '—')) ORDER BY (sd.name || ' / ' || COALESCE(dc.name, '—')))
         FROM product_placements pp
         JOIN store_departments sd ON sd.id = pp.department_id
         LEFT JOIN department_categories dc ON dc.id = pp.category_id
         WHERE pp.product_id = p.id AND pp.active AND sd.branch_id = b.id
       ), '{}') AS placement_labels
FROM stock_balances sb
JOIN warehouses w ON w.id = sb.warehouse_id
JOIN branches b ON b.id = w.branch_id
JOIN product_skus ps ON ps.id = sb.sku_id
JOIN products p ON p.id = ps.product_id
WHERE 1=1`
	args := []any{}
	argN := 1
	if filter.BranchCode != "" {
		q += fmt.Sprintf(` AND b.code = $%d`, argN)
		args = append(args, filter.BranchCode)
		argN++
	}
	if filter.DepartmentCode != "" {
		q += fmt.Sprintf(` AND EXISTS (
  SELECT 1 FROM product_placements pp
  JOIN store_departments sd ON sd.id = pp.department_id
  WHERE pp.product_id = p.id AND pp.active AND sd.branch_id = b.id AND sd.code = $%d
)`, argN)
		args = append(args, filter.DepartmentCode)
		argN++
	}
	if filter.CategoryCode != "" {
		q += fmt.Sprintf(` AND EXISTS (
  SELECT 1 FROM product_placements pp
  JOIN department_categories dc ON dc.id = pp.category_id
  JOIN store_departments sd ON sd.id = pp.department_id
  WHERE pp.product_id = p.id AND pp.active AND sd.branch_id = b.id AND dc.code = $%d
)`, argN)
		args = append(args, filter.CategoryCode)
		argN++
	}
	q += ` ORDER BY b.code, w.code, ps.sku`

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.StockBalance
	for rows.Next() {
		var b domain.StockBalance
		if err := rows.Scan(
			&b.ID, &b.WarehouseID, &b.BranchID, &b.SKUID, &b.SKU, &b.ProductName,
			&b.OnHand, &b.Reserved, &b.Version,
			&b.Departments, &b.Categories, &b.Placements,
		); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Postgres) ListDepartments(ctx context.Context, orgRef, branchCode string) ([]domain.Department, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	q := `
SELECT sd.code, sd.name, b.code, sd.sort_order,
       COALESCE(dc.code, ''), COALESCE(dc.name, ''), COALESCE(dc.sort_order, 0)
FROM store_departments sd
JOIN branches b ON b.id = sd.branch_id
LEFT JOIN department_categories dc ON dc.department_id = sd.id AND dc.active = TRUE
WHERE sd.active = TRUE`
	args := []any{}
	if branchCode != "" {
		q += ` AND b.code = $1`
		args = append(args, branchCode)
	}
	q += ` ORDER BY sd.sort_order, sd.code, dc.sort_order, dc.code`

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byCode := map[string]*domain.Department{}
	var order []string
	for rows.Next() {
		var deptCode, deptName, branchID, catCode, catName string
		var deptSort, catSort int
		if err := rows.Scan(&deptCode, &deptName, &branchID, &deptSort, &catCode, &catName, &catSort); err != nil {
			return nil, err
		}
		d, ok := byCode[deptCode]
		if !ok {
			d = &domain.Department{
				Code: deptCode, Name: deptName, BranchID: branchID, SortOrder: deptSort,
				Categories: []domain.DepartmentCategory{},
			}
			byCode[deptCode] = d
			order = append(order, deptCode)
		}
		if catCode != "" {
			d.Categories = append(d.Categories, domain.DepartmentCategory{
				Code: catCode, Name: catName, SortOrder: catSort,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]domain.Department, 0, len(order))
	for _, code := range order {
		out = append(out, *byCode[code])
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Postgres) ListCatalog(ctx context.Context, filter domain.CatalogFilter) ([]domain.CatalogItem, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, filter.OrgRef)
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	q := `
SELECT DISTINCT p.id::text, p.sku_base, p.name,
       sd.code, sd.name,
       COALESCE(dc.code, ''), COALESCE(dc.name, ''),
       pp.is_primary
FROM products p
JOIN product_placements pp ON pp.product_id = p.id AND pp.active
JOIN store_departments sd ON sd.id = pp.department_id AND sd.active
JOIN branches b ON b.id = sd.branch_id
LEFT JOIN department_categories dc ON dc.id = pp.category_id
WHERE 1=1`
	args := []any{}
	argN := 1
	if filter.BranchCode != "" {
		q += fmt.Sprintf(` AND b.code = $%d`, argN)
		args = append(args, filter.BranchCode)
		argN++
	}
	if filter.DepartmentCode != "" {
		q += fmt.Sprintf(` AND sd.code = $%d`, argN)
		args = append(args, filter.DepartmentCode)
		argN++
	}
	if filter.CategoryCode != "" {
		q += fmt.Sprintf(` AND dc.code = $%d`, argN)
		args = append(args, filter.CategoryCode)
		argN++
	}
	q += ` ORDER BY 3, 8 DESC, 4, 6`

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[string]*domain.CatalogItem{}
	var order []string
	for rows.Next() {
		var productID, skuBase, name, deptCode, deptName, catCode, catName string
		var isPrimary bool
		if err := rows.Scan(&productID, &skuBase, &name, &deptCode, &deptName, &catCode, &catName, &isPrimary); err != nil {
			return nil, err
		}
		item, ok := byID[productID]
		if !ok {
			item = &domain.CatalogItem{
				ProductID: productID, SKUBase: skuBase, Name: name,
				SKUs:       []string{},
				Placements: []domain.CatalogPlacement{},
			}
			byID[productID] = item
			order = append(order, productID)
		}
		item.Placements = append(item.Placements, domain.CatalogPlacement{
			DepartmentCode: deptCode,
			DepartmentName: deptName,
			CategoryCode:   catCode,
			CategoryName:   catName,
			IsPrimary:      isPrimary,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Attach SKUs in a second pass.
	for _, id := range order {
		skuRows, err := tx.Query(ctx, `SELECT sku FROM product_skus WHERE product_id = $1::uuid ORDER BY sku`, id)
		if err != nil {
			return nil, err
		}
		var skus []string
		for skuRows.Next() {
			var sku string
			if err := skuRows.Scan(&sku); err != nil {
				skuRows.Close()
				return nil, err
			}
			skus = append(skus, sku)
		}
		skuRows.Close()
		if err := skuRows.Err(); err != nil {
			return nil, err
		}
		byID[id].SKUs = skus
	}

	out := make([]domain.CatalogItem, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Postgres) ListLabels(ctx context.Context, filter domain.LabelFilter) ([]domain.StoreLabel, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, filter.OrgRef)
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	q := `
SELECT l.id::text, b.code,
       COALESCE(l.store_display_name, ''),
       ps.sku,
       COALESCE(ps.material_code, ps.sku),
       COALESCE(ps.barcode, ''),
       COALESCE(ps.size_code, ''),
       COALESCE(ps.color_code, ''),
       COALESCE(NULLIF(l.brand_label, ''), ps.brand, ''),
       l.public_description,
       COALESCE(l.department_label, ''),
       l.extra_descriptions,
       l.currency,
       l.common_price::float8,
       l.special_price::float8,
       l.final_price::float8,
       l.price_mode,
       (
         SELECT SUM(sb.on_hand)::float8
         FROM stock_balances sb
         JOIN warehouses w ON w.id = sb.warehouse_id
         WHERE sb.sku_id = ps.id AND w.branch_id = b.id
       )
FROM store_sku_labels l
JOIN branches b ON b.id = l.branch_id
JOIN product_skus ps ON ps.id = l.sku_id
WHERE l.active = TRUE`
	args := []any{}
	argN := 1
	if filter.BranchCode != "" {
		q += fmt.Sprintf(` AND b.code = $%d`, argN)
		args = append(args, filter.BranchCode)
		argN++
	}
	if filter.SKUCode != "" {
		q += fmt.Sprintf(` AND (ps.sku = $%d OR ps.material_code = $%d OR ps.barcode = $%d)`, argN, argN, argN)
		args = append(args, filter.SKUCode)
		argN++
	}
	if filter.DepartmentCode != "" {
		q += fmt.Sprintf(` AND EXISTS (
  SELECT 1 FROM product_placements pp
  JOIN store_departments sd ON sd.id = pp.department_id
  JOIN products p ON p.id = pp.product_id
  WHERE p.id = ps.product_id AND pp.active AND sd.branch_id = b.id AND sd.code = $%d
)`, argN)
		args = append(args, filter.DepartmentCode)
		argN++
	}
	q += ` ORDER BY b.code, ps.sku`

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.StoreLabel
	for rows.Next() {
		var l domain.StoreLabel
		var extraRaw []byte
		var common, special, final, onHand *float64
		if err := rows.Scan(
			&l.ID, &l.BranchID, &l.StoreDisplayName, &l.SKU, &l.MaterialCode, &l.Barcode,
			&l.SizeCode, &l.ColorCode, &l.Brand, &l.PublicDescription, &l.DepartmentLabel,
			&extraRaw, &l.Currency, &common, &special, &final, &l.PriceMode, &onHand,
		); err != nil {
			return nil, err
		}
		l.CommonPrice = common
		l.SpecialPrice = special
		l.FinalPrice = final
		l.OnHand = onHand
		if len(extraRaw) > 0 {
			_ = json.Unmarshal(extraRaw, &l.ExtraDescriptions)
		}
		l.EffectivePrice = l.ResolveEffectivePrice()
		l.PriceLabel = priceLabel(l.PriceMode)
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Postgres) PostMovement(ctx context.Context, req domain.MovementRequest) (domain.Movement, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, req.OrgID)
	})
	if err != nil {
		return domain.Movement{}, err
	}
	defer tx.Rollback(ctx)

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
		// Inbound movements may create the first balance row (CEDI / arrival receiving).
		if delta > 0 {
			balID = uuid.New().String()
			_, err = tx.Exec(ctx, `
INSERT INTO stock_balances (id, warehouse_id, sku_id, on_hand, reserved, version, updated_at)
VALUES ($1::uuid, $2::uuid, $3::uuid, 0, 0, 1, now())`, balID, warehouseID, skuID)
			if err != nil {
				return domain.Movement{}, err
			}
			onHand = 0
			version = 1
		} else {
			return domain.Movement{}, domain.ErrNotFound
		}
	} else if err != nil {
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
  'org_id', $2::text,
  'branch_id', $3::text,
  'warehouse_id', $4::text,
  'sku_id', $5::text,
  'quantity', $6::float8
))`, movID.String(), orgID, req.BranchID, req.WarehouseID, req.SKUID, delta)

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

func (s *Postgres) ListMovements(ctx context.Context, filter domain.MovementFilter) ([]domain.Movement, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, filter.OrgRef)
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	q := `
SELECT m.id::text, m.org_id::text, b.code, w.code, ps.sku,
       m.movement_type, m.quantity::float8, m.status,
       COALESCE(u.idp_sub, COALESCE(m.posted_by::text, '')),
       m.idempotency_key, m.created_at,
       COALESCE(m.reversal_of::text, ''),
       COALESCE(m.void_reason, ''),
       COALESCE(vu.idp_sub, COALESCE(m.voided_by::text, '')),
       m.voided_at
FROM inventory_movements m
JOIN branches b ON b.id = m.branch_id
JOIN warehouses w ON w.id = m.warehouse_id
JOIN product_skus ps ON ps.id = m.sku_id
LEFT JOIN users u ON u.id = m.posted_by
LEFT JOIN users vu ON vu.id = m.voided_by
WHERE 1=1`
	args := []any{}
	argN := 1
	if filter.BranchCode != "" {
		q += fmt.Sprintf(` AND b.code = $%d`, argN)
		args = append(args, filter.BranchCode)
		argN++
	}
	if filter.WarehouseID != "" {
		q += fmt.Sprintf(` AND w.code = $%d`, argN)
		args = append(args, filter.WarehouseID)
		argN++
	}
	q += fmt.Sprintf(` ORDER BY m.created_at DESC LIMIT $%d`, argN)
	args = append(args, limit)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Movement
	for rows.Next() {
		var m domain.Movement
		var voidedAt *time.Time
		if err := rows.Scan(
			&m.ID, &m.OrgID, &m.BranchID, &m.WarehouseID, &m.SKUID,
			&m.MovementType, &m.Quantity, &m.Status, &m.PostedBy,
			&m.IdempotencyKey, &m.CreatedAt, &m.ReversalOf, &m.VoidReason, &m.VoidedBy, &voidedAt,
		); err != nil {
			return nil, err
		}
		m.VoidedAt = voidedAt
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Postgres) GetMovement(ctx context.Context, orgRef, movementID string) (domain.Movement, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.Movement{}, err
	}
	defer tx.Rollback(ctx)

	m, err := scanMovementByID(ctx, tx, movementID)
	if err != nil {
		return domain.Movement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Movement{}, err
	}
	return m, nil
}

func (s *Postgres) VoidMovement(ctx context.Context, movementID string, req domain.VoidRequest) (domain.VoidResult, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, req.OrgID)
	})
	if err != nil {
		return domain.VoidResult{}, err
	}
	defer tx.Rollback(ctx)

	if req.Reason == "" {
		return domain.VoidResult{}, errors.New("reason required")
	}
	if req.IdempotencyKey == "" {
		return domain.VoidResult{}, errors.New("idempotency_key required")
	}

	// Idempotent replay: compensation already created for this key.
	var existingComp domain.Movement
	err = tx.QueryRow(ctx, `
SELECT id::text FROM inventory_movements WHERE org_id = $1::uuid AND idempotency_key = $2`,
		orgID, req.IdempotencyKey).Scan(&existingComp.ID)
	if err == nil {
		comp, err := scanMovementByID(ctx, tx, existingComp.ID)
		if err != nil {
			return domain.VoidResult{}, err
		}
		orig, err := scanMovementByID(ctx, tx, movementID)
		if err != nil {
			return domain.VoidResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.VoidResult{}, err
		}
		return domain.VoidResult{Original: orig, Compensation: comp}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.VoidResult{}, err
	}

	var warehouseUUID, skuUUID, branchUUID string
	var qty float64
	var status string
	err = tx.QueryRow(ctx, `
SELECT warehouse_id::text, sku_id::text, branch_id::text, quantity::float8, status
FROM inventory_movements
WHERE id = $1::uuid AND org_id = $2::uuid
FOR UPDATE`, movementID, orgID).Scan(&warehouseUUID, &skuUUID, &branchUUID, &qty, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VoidResult{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.VoidResult{}, err
	}
	if status == "VOID" {
		return domain.VoidResult{}, domain.ErrAlreadyVoided
	}
	if status != "POSTED" {
		return domain.VoidResult{}, errors.New("only posted movements can be voided")
	}

	voidedBy, err := resolveUserID(ctx, tx, orgID, req.VoidedBy)
	if err != nil {
		return domain.VoidResult{}, err
	}

	var balID string
	var onHand float64
	var version int
	err = tx.QueryRow(ctx, `
SELECT id::text, on_hand::float8, version
FROM stock_balances
WHERE warehouse_id = $1::uuid AND sku_id = $2::uuid
FOR UPDATE`, warehouseUUID, skuUUID).Scan(&balID, &onHand, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VoidResult{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.VoidResult{}, err
	}

	compDelta := -qty
	next := onHand + compDelta
	if next < 0 {
		return domain.VoidResult{}, domain.ErrInsufficientStock
	}

	ct, err := tx.Exec(ctx, `
UPDATE stock_balances
SET on_hand = $1, version = version + 1, updated_at = now()
WHERE id = $2::uuid AND version = $3`, next, balID, version)
	if err != nil {
		return domain.VoidResult{}, err
	}
	if ct.RowsAffected() == 0 {
		return domain.VoidResult{}, domain.ErrConflict
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
UPDATE inventory_movements
SET status = 'VOID', void_reason = $1, voided_by = $2, voided_at = $3
WHERE id = $4::uuid`, req.Reason, voidedBy, now, movementID)
	if err != nil {
		return domain.VoidResult{}, err
	}

	compID := uuid.New()
	_, err = tx.Exec(ctx, `
INSERT INTO inventory_movements (
  id, org_id, branch_id, sku_id, warehouse_id, movement_type, quantity, status,
  posted_by, idempotency_key, created_at, reversal_of
) VALUES ($1,$2,$3,$4,$5,'REVERSAL',$6,'POSTED',$7,$8,$9,$10::uuid)`,
		compID, orgID, branchUUID, skuUUID, warehouseUUID, compDelta, voidedBy, req.IdempotencyKey, now, movementID)
	if err != nil {
		return domain.VoidResult{}, err
	}

	orig, err := scanMovementByID(ctx, tx, movementID)
	if err != nil {
		return domain.VoidResult{}, err
	}
	comp, err := scanMovementByID(ctx, tx, compID.String())
	if err != nil {
		return domain.VoidResult{}, err
	}

	_, _ = tx.Exec(ctx, `
INSERT INTO outbox (event_type, payload) VALUES ('InventoryMovedVoided', jsonb_build_object(
  'movement_id', $1::text,
  'compensation_id', $2::text,
  'org_id', $3::text,
  'branch_id', $4::text,
  'warehouse_id', $5::text,
  'sku_id', $6::text,
  'quantity', $7::float8,
  'reason', $8::text,
  'voided_by', $9::text
))`, orig.ID, comp.ID, orgID, orig.BranchID, orig.WarehouseID, orig.SKUID, compDelta, req.Reason, req.VoidedBy)

	if err := tx.Commit(ctx); err != nil {
		return domain.VoidResult{}, err
	}
	return domain.VoidResult{Original: orig, Compensation: comp}, nil
}

func scanMovementByID(ctx context.Context, tx pgx.Tx, movementID string) (domain.Movement, error) {
	var m domain.Movement
	var voidedAt *time.Time
	err := tx.QueryRow(ctx, `
SELECT m.id::text, m.org_id::text, b.code, w.code, ps.sku,
       m.movement_type, m.quantity::float8, m.status,
       COALESCE(u.idp_sub, COALESCE(m.posted_by::text, '')),
       m.idempotency_key, m.created_at,
       COALESCE(m.reversal_of::text, ''),
       COALESCE(m.void_reason, ''),
       COALESCE(vu.idp_sub, COALESCE(m.voided_by::text, '')),
       m.voided_at
FROM inventory_movements m
JOIN branches b ON b.id = m.branch_id
JOIN warehouses w ON w.id = m.warehouse_id
JOIN product_skus ps ON ps.id = m.sku_id
LEFT JOIN users u ON u.id = m.posted_by
LEFT JOIN users vu ON vu.id = m.voided_by
WHERE m.id = $1::uuid`, movementID).Scan(
		&m.ID, &m.OrgID, &m.BranchID, &m.WarehouseID, &m.SKUID,
		&m.MovementType, &m.Quantity, &m.Status, &m.PostedBy,
		&m.IdempotencyKey, &m.CreatedAt, &m.ReversalOf, &m.VoidReason, &m.VoidedBy, &voidedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Movement{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Movement{}, err
	}
	m.VoidedAt = voidedAt
	return m, nil
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
