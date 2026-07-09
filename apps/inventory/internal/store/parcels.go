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

func (s *Postgres) ShipShippingSlip(ctx context.Context, orgRef, slipID, actor string) (domain.ShippingSlip, error) {
	return s.transitionShippingSlip(ctx, orgRef, slipID, actor, domain.SlipStatusInTransit, "shipped_at")
}

func (s *Postgres) ReceiveShippingSlip(ctx context.Context, orgRef, slipID, actor string) (domain.ShippingSlip, error) {
	return s.transitionShippingSlip(ctx, orgRef, slipID, actor, domain.SlipStatusReceived, "received_at")
}

func (s *Postgres) CancelShippingSlip(ctx context.Context, orgRef, slipID string, req domain.CancelParcelRequest) (domain.ShippingSlip, error) {
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
	if status == domain.SlipStatusReceived {
		return domain.ShippingSlip{}, errors.New("cannot cancel received slip")
	}
	if status == domain.SlipStatusCancelled {
		_ = tx.Commit(ctx)
		return s.GetShippingSlip(ctx, orgRef, slipID)
	}
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
UPDATE shipping_slips
SET status = 'CANCELLED', cancelled_at = $1, cancel_reason = $2, updated_at = $1
WHERE id = $3::uuid`, now, strings.TrimSpace(req.Reason), slipID)
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	_ = req.Actor
	if err := tx.Commit(ctx); err != nil {
		return domain.ShippingSlip{}, err
	}
	return s.GetShippingSlip(ctx, orgRef, slipID)
}

func (s *Postgres) transitionShippingSlip(ctx context.Context, orgRef, slipID, actor, next, tsCol string) (domain.ShippingSlip, error) {
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
	if status == next {
		_ = tx.Commit(ctx)
		return s.GetShippingSlip(ctx, orgRef, slipID)
	}
	allowed := map[string][]string{
		domain.SlipStatusInTransit: {domain.SlipStatusDraft, domain.SlipStatusPrinted},
		domain.SlipStatusReceived:  {domain.SlipStatusInTransit, domain.SlipStatusPrinted, domain.SlipStatusDraft},
	}
	ok := false
	for _, a := range allowed[next] {
		if status == a {
			ok = true
			break
		}
	}
	if !ok {
		return domain.ShippingSlip{}, fmt.Errorf("cannot move slip from %s to %s", status, next)
	}
	now := time.Now().UTC()
	q := fmt.Sprintf(`
UPDATE shipping_slips
SET status = $1, %s = COALESCE(%s, $2), updated_at = $2
WHERE id = $3::uuid`, tsCol, tsCol)
	_, err = tx.Exec(ctx, q, next, now, slipID)
	if err != nil {
		return domain.ShippingSlip{}, err
	}
	_ = actor
	if err := tx.Commit(ctx); err != nil {
		return domain.ShippingSlip{}, err
	}
	return s.GetShippingSlip(ctx, orgRef, slipID)
}

func (s *Postgres) DepartTransportSheet(ctx context.Context, orgRef, sheetID, actor string) (domain.TransportSheet, error) {
	return s.transitionTransportSheet(ctx, orgRef, sheetID, actor, domain.TransportStatusInTransit, "departed_at")
}

func (s *Postgres) DeliverTransportSheet(ctx context.Context, orgRef, sheetID, actor string) (domain.TransportSheet, error) {
	return s.transitionTransportSheet(ctx, orgRef, sheetID, actor, domain.TransportStatusDelivered, "delivered_at")
}

func (s *Postgres) CancelTransportSheet(ctx context.Context, orgRef, sheetID string, req domain.CancelParcelRequest) (domain.TransportSheet, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.TransportSheet{}, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM transport_sheets WHERE id = $1::uuid FOR UPDATE`, sheetID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TransportSheet{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TransportSheet{}, err
	}
	if status == domain.TransportStatusDelivered {
		return domain.TransportSheet{}, errors.New("cannot cancel delivered sheet")
	}
	if status == domain.TransportStatusCancelled {
		_ = tx.Commit(ctx)
		return s.GetTransportSheet(ctx, orgRef, sheetID)
	}
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
UPDATE transport_sheets
SET status = 'CANCELLED', cancelled_at = $1, cancel_reason = $2, updated_at = $1
WHERE id = $3::uuid`, now, strings.TrimSpace(req.Reason), sheetID)
	if err != nil {
		return domain.TransportSheet{}, err
	}
	_ = req.Actor
	if err := tx.Commit(ctx); err != nil {
		return domain.TransportSheet{}, err
	}
	return s.GetTransportSheet(ctx, orgRef, sheetID)
}

func (s *Postgres) transitionTransportSheet(ctx context.Context, orgRef, sheetID, actor, next, tsCol string) (domain.TransportSheet, error) {
	tx, _, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.TransportSheet{}, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM transport_sheets WHERE id = $1::uuid FOR UPDATE`, sheetID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TransportSheet{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TransportSheet{}, err
	}
	if status == domain.TransportStatusCancelled {
		return domain.TransportSheet{}, errors.New("sheet cancelled")
	}
	if status == next {
		_ = tx.Commit(ctx)
		return s.GetTransportSheet(ctx, orgRef, sheetID)
	}
	allowed := map[string][]string{
		domain.TransportStatusInTransit: {domain.TransportStatusDraft, domain.TransportStatusPrinted},
		domain.TransportStatusDelivered: {domain.TransportStatusInTransit, domain.TransportStatusPrinted},
	}
	ok := false
	for _, a := range allowed[next] {
		if status == a {
			ok = true
			break
		}
	}
	if !ok {
		return domain.TransportSheet{}, fmt.Errorf("cannot move sheet from %s to %s", status, next)
	}
	now := time.Now().UTC()
	q := fmt.Sprintf(`
UPDATE transport_sheets
SET status = $1, %s = COALESCE(%s, $2), updated_at = $2
WHERE id = $3::uuid`, tsCol, tsCol)
	_, err = tx.Exec(ctx, q, next, now, sheetID)
	if err != nil {
		return domain.TransportSheet{}, err
	}
	// Advance linked slips when departing / delivering.
	if next == domain.TransportStatusInTransit {
		_, _ = tx.Exec(ctx, `
UPDATE shipping_slips s
SET status = 'IN_TRANSIT', shipped_at = COALESCE(s.shipped_at, $1), updated_at = $1
FROM transport_sheet_slip_links l
JOIN transport_sheet_sections sec ON sec.id = l.section_id
WHERE sec.sheet_id = $2::uuid AND l.shipping_slip_id = s.id
  AND s.status IN ('DRAFT', 'PRINTED')`, now, sheetID)
	}
	if next == domain.TransportStatusDelivered {
		_, _ = tx.Exec(ctx, `
UPDATE shipping_slips s
SET status = 'RECEIVED', received_at = COALESCE(s.received_at, $1), updated_at = $1
FROM transport_sheet_slip_links l
JOIN transport_sheet_sections sec ON sec.id = l.section_id
WHERE sec.sheet_id = $2::uuid AND l.shipping_slip_id = s.id
  AND s.status IN ('DRAFT', 'PRINTED', 'IN_TRANSIT')`, now, sheetID)
	}
	_ = actor
	if err := tx.Commit(ctx); err != nil {
		return domain.TransportSheet{}, err
	}
	return s.GetTransportSheet(ctx, orgRef, sheetID)
}

func (s *Postgres) CreateWarrantyCase(ctx context.Context, req domain.CreateWarrantyCaseRequest) (domain.WarrantyCase, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return domain.WarrantyCase{}, errors.New("idempotency_key required")
	}
	if strings.TrimSpace(req.BranchID) == "" {
		return domain.WarrantyCase{}, errors.New("branch_id required")
	}
	if strings.TrimSpace(req.ProblemDescription) == "" {
		return domain.WarrantyCase{}, errors.New("problem_description required")
	}
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, req.OrgID)
	})
	if err != nil {
		return domain.WarrantyCase{}, err
	}
	defer tx.Rollback(ctx)

	var existing string
	err = tx.QueryRow(ctx, `
SELECT id::text FROM warranty_cases WHERE org_id = $1::uuid AND idempotency_key = $2`,
		orgID, req.IdempotencyKey).Scan(&existing)
	if err == nil {
		_ = tx.Commit(ctx)
		return s.GetWarrantyCase(ctx, req.OrgID, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.WarrantyCase{}, err
	}

	branchID, err := resolveBranchID(ctx, tx, orgID, req.BranchID)
	if err != nil {
		return domain.WarrantyCase{}, err
	}
	var dest any
	if strings.TrimSpace(req.DestinationBranchID) != "" {
		d, err := resolveBranchID(ctx, tx, orgID, req.DestinationBranchID)
		if err != nil {
			return domain.WarrantyCase{}, err
		}
		dest = d
	} else {
		// Default destination: CEDI branch if present.
		var cedi string
		_ = tx.QueryRow(ctx, `
SELECT id::text FROM branches WHERE org_id = $1::uuid AND (branch_kind = 'CEDI' OR code = 'br_cedi') LIMIT 1`,
			orgID).Scan(&cedi)
		if cedi != "" {
			dest = cedi
		}
	}
	var skuID any
	skuCode := strings.TrimSpace(req.SKU)
	if skuCode != "" {
		id, err := resolveSKUID(ctx, tx, skuCode)
		if err != nil {
			return domain.WarrantyCase{}, err
		}
		skuID = id
	}

	id := uuid.New()
	now := time.Now().UTC()
	number := fmt.Sprintf("GAR-%s", strings.ToUpper(id.String()[:8]))
	_, err = tx.Exec(ctx, `
INSERT INTO warranty_cases (
  id, org_id, case_number, branch_id, destination_branch_id, sku_id, sku_code,
  serial_number, customer_ref, problem_description, status, parcel_kind,
  created_by, operator_label, idempotency_key, created_at, updated_at
) VALUES (
  $1, $2::uuid, $3, $4::uuid, $5, $6, $7,
  $8, $9, $10, 'OPEN', 'WARRANTY',
  $11, $12, $13, $14, $14
)`, id, orgID, number, branchID, dest, skuID, skuCode,
		strings.TrimSpace(req.SerialNumber), strings.TrimSpace(req.CustomerRef),
		strings.TrimSpace(req.ProblemDescription), req.CreatedBy, strings.TrimSpace(req.OperatorLabel),
		req.IdempotencyKey, now)
	if err != nil {
		return domain.WarrantyCase{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.WarrantyCase{}, err
	}
	return s.GetWarrantyCase(ctx, req.OrgID, id.String())
}

func (s *Postgres) ListWarrantyCases(ctx context.Context, filter domain.WarrantyCaseFilter) ([]domain.WarrantyCase, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	var out []domain.WarrantyCase
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}
		q := `
SELECT c.id::text, c.org_id::text, c.case_number, b.code,
       COALESCE(db.code, ''), COALESCE(c.sku_code, ''), c.serial_number, c.customer_ref,
       c.problem_description, c.status, c.parcel_kind,
       COALESCE(c.shipping_slip_id::text, ''), COALESCE(c.transfer_id::text, ''),
       c.created_by, c.operator_label, c.closed_at, c.idempotency_key, c.created_at
FROM warranty_cases c
JOIN branches b ON b.id = c.branch_id
LEFT JOIN branches db ON db.id = c.destination_branch_id
WHERE c.org_id = $1::uuid`
		args := []any{orgID}
		n := 2
		if filter.BranchID != "" {
			q += fmt.Sprintf(` AND b.code = $%d`, n)
			args = append(args, filter.BranchID)
			n++
		}
		if filter.Status != "" {
			q += fmt.Sprintf(` AND c.status = $%d`, n)
			args = append(args, strings.ToUpper(filter.Status))
			n++
		}
		q += fmt.Sprintf(` ORDER BY c.created_at DESC LIMIT $%d`, n)
		args = append(args, limit)
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c domain.WarrantyCase
			if err := rows.Scan(
				&c.ID, &c.OrgID, &c.CaseNumber, &c.BranchID, &c.DestinationBranchID,
				&c.SKU, &c.SerialNumber, &c.CustomerRef, &c.ProblemDescription,
				&c.Status, &c.ParcelKind, &c.ShippingSlipID, &c.TransferID,
				&c.CreatedBy, &c.OperatorLabel, &c.ClosedAt, &c.IdempotencyKey, &c.CreatedAt,
			); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Postgres) GetWarrantyCase(ctx context.Context, orgRef, caseID string) (domain.WarrantyCase, error) {
	var out domain.WarrantyCase
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
SELECT c.id::text, c.org_id::text, c.case_number, b.code,
       COALESCE(db.code, ''), COALESCE(c.sku_code, ''), c.serial_number, c.customer_ref,
       c.problem_description, c.status, c.parcel_kind,
       COALESCE(c.shipping_slip_id::text, ''), COALESCE(c.transfer_id::text, ''),
       c.created_by, c.operator_label, c.closed_at, c.idempotency_key, c.created_at
FROM warranty_cases c
JOIN branches b ON b.id = c.branch_id
LEFT JOIN branches db ON db.id = c.destination_branch_id
WHERE c.org_id = $1::uuid AND c.id = $2::uuid`, orgID, caseID).Scan(
			&out.ID, &out.OrgID, &out.CaseNumber, &out.BranchID, &out.DestinationBranchID,
			&out.SKU, &out.SerialNumber, &out.CustomerRef, &out.ProblemDescription,
			&out.Status, &out.ParcelKind, &out.ShippingSlipID, &out.TransferID,
			&out.CreatedBy, &out.OperatorLabel, &out.ClosedAt, &out.IdempotencyKey, &out.CreatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	return out, err
}

func (s *Postgres) CreateReturnCase(ctx context.Context, req domain.CreateReturnCaseRequest) (domain.ReturnCase, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return domain.ReturnCase{}, errors.New("idempotency_key required")
	}
	if strings.TrimSpace(req.FromBranchID) == "" || strings.TrimSpace(req.ToBranchID) == "" {
		return domain.ReturnCase{}, errors.New("from_branch_id and to_branch_id required")
	}
	reason := strings.ToUpper(strings.TrimSpace(req.Reason))
	if reason == "" {
		reason = domain.ReturnReasonDefective
	}
	parcelKind := domain.ParcelDefective
	switch reason {
	case domain.ReturnReasonWarranty:
		parcelKind = domain.ParcelWarranty
	case domain.ReturnReasonCustomerReturn, domain.ReturnReasonOverstock:
		parcelKind = domain.ParcelReturnToCEDI
	case domain.ReturnReasonDefective, domain.ReturnReasonOther:
		parcelKind = domain.ParcelDefective
	default:
		return domain.ReturnCase{}, errors.New("invalid reason")
	}

	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, req.OrgID)
	})
	if err != nil {
		return domain.ReturnCase{}, err
	}
	defer tx.Rollback(ctx)

	var existing string
	err = tx.QueryRow(ctx, `
SELECT id::text FROM return_cases WHERE org_id = $1::uuid AND idempotency_key = $2`,
		orgID, req.IdempotencyKey).Scan(&existing)
	if err == nil {
		_ = tx.Commit(ctx)
		return s.GetReturnCase(ctx, req.OrgID, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.ReturnCase{}, err
	}

	fromBranch, err := resolveBranchID(ctx, tx, orgID, req.FromBranchID)
	if err != nil {
		return domain.ReturnCase{}, err
	}
	toBranch, err := resolveBranchID(ctx, tx, orgID, req.ToBranchID)
	if err != nil {
		return domain.ReturnCase{}, err
	}

	id := uuid.New()
	now := time.Now().UTC()
	number := fmt.Sprintf("DEV-%s", strings.ToUpper(id.String()[:8]))
	_, err = tx.Exec(ctx, `
INSERT INTO return_cases (
  id, org_id, case_number, from_branch_id, to_branch_id, reason, notes, status, parcel_kind,
  created_by, operator_label, idempotency_key, created_at, updated_at
) VALUES (
  $1, $2::uuid, $3, $4::uuid, $5::uuid, $6, $7, 'OPEN', $8,
  $9, $10, $11, $12, $12
)`, id, orgID, number, fromBranch, toBranch, reason, strings.TrimSpace(req.Notes), parcelKind,
		req.CreatedBy, strings.TrimSpace(req.OperatorLabel), req.IdempotencyKey, now)
	if err != nil {
		return domain.ReturnCase{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ReturnCase{}, err
	}
	return s.GetReturnCase(ctx, req.OrgID, id.String())
}

func (s *Postgres) ListReturnCases(ctx context.Context, filter domain.ReturnCaseFilter) ([]domain.ReturnCase, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	var out []domain.ReturnCase
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}
		q := `
SELECT c.id::text, c.org_id::text, c.case_number, fb.code, tb.code,
       c.reason, c.notes, c.status, c.parcel_kind,
       COALESCE(c.shipping_slip_id::text, ''), COALESCE(c.transfer_id::text, ''),
       c.created_by, c.operator_label, c.closed_at, c.idempotency_key, c.created_at
FROM return_cases c
JOIN branches fb ON fb.id = c.from_branch_id
JOIN branches tb ON tb.id = c.to_branch_id
WHERE c.org_id = $1::uuid`
		args := []any{orgID}
		n := 2
		if filter.FromBranch != "" {
			q += fmt.Sprintf(` AND fb.code = $%d`, n)
			args = append(args, filter.FromBranch)
			n++
		}
		if filter.Status != "" {
			q += fmt.Sprintf(` AND c.status = $%d`, n)
			args = append(args, strings.ToUpper(filter.Status))
			n++
		}
		q += fmt.Sprintf(` ORDER BY c.created_at DESC LIMIT $%d`, n)
		args = append(args, limit)
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c domain.ReturnCase
			if err := rows.Scan(
				&c.ID, &c.OrgID, &c.CaseNumber, &c.FromBranchID, &c.ToBranchID,
				&c.Reason, &c.Notes, &c.Status, &c.ParcelKind, &c.ShippingSlipID, &c.TransferID,
				&c.CreatedBy, &c.OperatorLabel, &c.ClosedAt, &c.IdempotencyKey, &c.CreatedAt,
			); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Postgres) GetReturnCase(ctx context.Context, orgRef, caseID string) (domain.ReturnCase, error) {
	var out domain.ReturnCase
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
SELECT c.id::text, c.org_id::text, c.case_number, fb.code, tb.code,
       c.reason, c.notes, c.status, c.parcel_kind,
       COALESCE(c.shipping_slip_id::text, ''), COALESCE(c.transfer_id::text, ''),
       c.created_by, c.operator_label, c.closed_at, c.idempotency_key, c.created_at
FROM return_cases c
JOIN branches fb ON fb.id = c.from_branch_id
JOIN branches tb ON tb.id = c.to_branch_id
WHERE c.org_id = $1::uuid AND c.id = $2::uuid`, orgID, caseID).Scan(
			&out.ID, &out.OrgID, &out.CaseNumber, &out.FromBranchID, &out.ToBranchID,
			&out.Reason, &out.Notes, &out.Status, &out.ParcelKind, &out.ShippingSlipID, &out.TransferID,
			&out.CreatedBy, &out.OperatorLabel, &out.ClosedAt, &out.IdempotencyKey, &out.CreatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	return out, err
}

func (s *Postgres) ListParcelHub(ctx context.Context, filter domain.ParcelHubFilter) ([]domain.ParcelHubItem, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 80 {
		limit = 40
	}
	var out []domain.ParcelHubItem
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}
		kindFilter := strings.ToUpper(strings.TrimSpace(filter.ParcelKind))
		q := `
SELECT * FROM (
  SELECT 'slip'::text AS kind, s.id::text, s.slip_number AS number, s.parcel_kind, s.status,
         fb.code AS from_b, tb.code AS to_b, s.created_at
  FROM shipping_slips s
  JOIN branches fb ON fb.id = s.from_branch_id
  JOIN branches tb ON tb.id = s.to_branch_id
  WHERE s.org_id = $1::uuid
  UNION ALL
  SELECT 'transfer', t.id::text, t.transfer_number, t.parcel_kind, t.status,
         fb.code, tb.code, t.created_at
  FROM inventory_transfers t
  JOIN branches fb ON fb.id = t.from_branch_id
  JOIN branches tb ON tb.id = t.to_branch_id
  WHERE t.org_id = $1::uuid
  UNION ALL
  SELECT 'transport', h.id::text, h.sheet_number, h.parcel_kind, h.status,
         fb.code, tb.code, h.created_at
  FROM transport_sheets h
  JOIN branches fb ON fb.id = h.from_branch_id
  JOIN branches tb ON tb.id = h.to_branch_id
  WHERE h.org_id = $1::uuid
  UNION ALL
  SELECT 'warranty', c.id::text, c.case_number, c.parcel_kind, c.status,
         b.code, COALESCE(db.code, ''), c.created_at
  FROM warranty_cases c
  JOIN branches b ON b.id = c.branch_id
  LEFT JOIN branches db ON db.id = c.destination_branch_id
  WHERE c.org_id = $1::uuid
  UNION ALL
  SELECT 'return', r.id::text, r.case_number, r.parcel_kind, r.status,
         fb.code, tb.code, r.created_at
  FROM return_cases r
  JOIN branches fb ON fb.id = r.from_branch_id
  JOIN branches tb ON tb.id = r.to_branch_id
  WHERE r.org_id = $1::uuid
) x WHERE 1=1`
		args := []any{orgID}
		n := 2
		if filter.BranchID != "" {
			q += fmt.Sprintf(` AND (x.from_b = $%d OR x.to_b = $%d)`, n, n)
			args = append(args, filter.BranchID)
			n++
		}
		if kindFilter != "" {
			q += fmt.Sprintf(` AND x.parcel_kind = $%d`, n)
			args = append(args, kindFilter)
			n++
		}
		q += fmt.Sprintf(` ORDER BY x.created_at DESC LIMIT $%d`, n)
		args = append(args, limit)

		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item domain.ParcelHubItem
			if err := rows.Scan(
				&item.Kind, &item.ID, &item.Number, &item.ParcelKind, &item.Status,
				&item.FromBranchID, &item.ToBranchID, &item.CreatedAt,
			); err != nil {
				return err
			}
			item.ParcelKindLabel = domain.ParcelKindLabelES(item.ParcelKind)
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}
