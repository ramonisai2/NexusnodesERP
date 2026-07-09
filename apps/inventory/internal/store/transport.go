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
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

func (s *Postgres) CreateTransportSheet(ctx context.Context, req domain.CreateTransportSheetRequest) (domain.TransportSheet, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return domain.TransportSheet{}, errors.New("idempotency_key required")
	}
	if strings.TrimSpace(req.FromBranchID) == "" || strings.TrimSpace(req.ToBranchID) == "" {
		return domain.TransportSheet{}, errors.New("from_branch_id and to_branch_id required")
	}
	if req.FromBranchID == req.ToBranchID {
		return domain.TransportSheet{}, errors.New("from and to branch must differ")
	}
	parcelKind := strings.ToUpper(strings.TrimSpace(req.ParcelKind))
	if parcelKind == "" {
		parcelKind = domain.ParcelTransfer
	}
	if !domain.ValidParcelKind(parcelKind) {
		return domain.TransportSheet{}, errors.New("invalid parcel_kind")
	}
	if len(req.Sections) == 0 {
		return domain.TransportSheet{}, errors.New("at least one department section required")
	}

	var sheetID string
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, req.OrgID)
		if err != nil {
			return err
		}
		if err := db.SetOrgContext(ctx, tx, orgID); err != nil {
			return err
		}

		var existingID string
		err = tx.QueryRow(ctx, `
SELECT id::text FROM transport_sheets WHERE org_id = $1::uuid AND idempotency_key = $2`,
			orgID, req.IdempotencyKey).Scan(&existingID)
		if err == nil {
			sheetID = existingID
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		fromBranch, err := resolveBranchID(ctx, tx, orgID, req.FromBranchID)
		if err != nil {
			return err
		}
		toBranch, err := resolveBranchID(ctx, tx, orgID, req.ToBranchID)
		if err != nil {
			return err
		}
		var sess any
		if req.SessionID != "" {
			if _, err := uuid.Parse(req.SessionID); err == nil {
				sess = req.SessionID
			}
		}

		id := uuid.New()
		now := time.Now().UTC()
		sheetNumber := fmt.Sprintf("HT-%s", strings.ToUpper(id.String()[:8]))

		sealNumber := strings.TrimSpace(req.SealNumber)
		sealStatus := ""
		if sealNumber != "" {
			sealStatus = domain.SealApplied
		}
		_, err = tx.Exec(ctx, `
INSERT INTO transport_sheets (
  id, org_id, sheet_number, from_branch_id, to_branch_id,
  carrier_name, vehicle_ref, driver_name, parcel_kind, status,
  created_by, operator_label, session_id, notes, idempotency_key, created_at, updated_at,
  seal_number, seal_status
) VALUES (
  $1, $2::uuid, $3, $4::uuid, $5::uuid,
  $6, $7, $8, $9, 'DRAFT',
  $10, $11, $12, $13, $14, $15, $15,
  $16, $17
)`, id, orgID, sheetNumber, fromBranch, toBranch,
			strings.TrimSpace(req.CarrierName), strings.TrimSpace(req.VehicleRef), strings.TrimSpace(req.DriverName),
			parcelKind, req.CreatedBy, strings.TrimSpace(req.OperatorLabel), sess,
			strings.TrimSpace(req.Notes), req.IdempotencyKey, now, sealNumber, sealStatus)
		if err != nil {
			return err
		}

		for i, sec := range req.Sections {
			deptCode := strings.TrimSpace(sec.DepartmentCode)
			if deptCode == "" {
				return errors.New("department_code required on each section")
			}
			deptName := strings.TrimSpace(sec.DepartmentName)
			if deptName == "" {
				err = tx.QueryRow(ctx, `
SELECT sd.name FROM store_departments sd
WHERE sd.branch_id = $1::uuid AND sd.code = $2 AND sd.active
LIMIT 1`, fromBranch, deptCode).Scan(&deptName)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
			if deptName == "" {
				deptName = deptCode
			}
			notes := strings.TrimSpace(sec.Notes)
			if notes == "" && len(sec.SlipIDs) == 0 {
				return fmt.Errorf("section %s needs notes or slip_ids", deptCode)
			}
			secID := uuid.New()
			_, err = tx.Exec(ctx, `
INSERT INTO transport_sheet_sections (id, sheet_id, department_code, department_name, notes, sort_order)
VALUES ($1, $2, $3, $4, $5, $6)`, secID, id, deptCode, deptName, notes, i)
			if err != nil {
				return err
			}
			for _, slipID := range sec.SlipIDs {
				slipID = strings.TrimSpace(slipID)
				if slipID == "" {
					continue
				}
				if _, err := uuid.Parse(slipID); err != nil {
					return fmt.Errorf("invalid slip_id %s", slipID)
				}
				var ok string
				err = tx.QueryRow(ctx, `
SELECT id::text FROM shipping_slips
WHERE id = $1::uuid AND org_id = $2::uuid AND from_branch_id = $3::uuid`,
					slipID, orgID, fromBranch).Scan(&ok)
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("slip not found for branch: %s", slipID)
				}
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, `
INSERT INTO transport_sheet_slip_links (id, section_id, shipping_slip_id)
VALUES ($1, $2, $3::uuid)
ON CONFLICT (section_id, shipping_slip_id) DO NOTHING`, uuid.New(), secID, slipID)
				if err != nil {
					return err
				}
			}
		}
		sheetID = id.String()
		return nil
	})
	if err != nil {
		return domain.TransportSheet{}, err
	}
	return s.GetTransportSheet(ctx, req.OrgID, sheetID)
}

func (s *Postgres) ListTransportSheets(ctx context.Context, filter domain.TransportSheetFilter) ([]domain.TransportSheet, error) {
	var out []domain.TransportSheet
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		if _, err := resolveOrgID(ctx, tx, filter.OrgRef); err != nil {
			return err
		}
		limit := filter.Limit
		if limit <= 0 || limit > 100 {
			limit = 40
		}

		q := `
SELECT t.id::text, t.org_id::text, t.sheet_number,
       fb.code, tb.code,
       t.carrier_name, t.vehicle_ref, t.driver_name, COALESCE(t.parcel_kind,'TRANSFER'), t.status,
       t.printed_at, t.departed_at, t.delivered_at, t.cancelled_at, COALESCE(t.cancel_reason,''),
       t.created_by, t.operator_label, COALESCE(t.session_id::text, ''),
       t.notes, t.idempotency_key, t.created_at, t.updated_at,
       COALESCE(t.seal_number,''), COALESCE(t.seal_status,''), t.seal_verified_at,
       COALESCE(vu.idp_sub, COALESCE(t.seal_verified_by::text, '')), COALESCE(t.seal_notes,'')
FROM transport_sheets t
JOIN branches fb ON fb.id = t.from_branch_id
JOIN branches tb ON tb.id = t.to_branch_id
LEFT JOIN users vu ON vu.id = t.seal_verified_by
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
			q += fmt.Sprintf(` AND t.status = $%d`, n)
			args = append(args, strings.ToUpper(filter.Status))
			n++
		}
		if filter.ParcelKind != "" {
			q += fmt.Sprintf(` AND t.parcel_kind = $%d`, n)
			args = append(args, strings.ToUpper(filter.ParcelKind))
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
			sheet, err := scanTransportSheet(rows)
			if err != nil {
				return err
			}
			out = append(out, sheet)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Postgres) GetTransportSheet(ctx context.Context, orgRef, sheetID string) (domain.TransportSheet, error) {
	var sheet domain.TransportSheet
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		_ = orgID
		row := tx.QueryRow(ctx, `
SELECT t.id::text, t.org_id::text, t.sheet_number,
       fb.code, tb.code,
       t.carrier_name, t.vehicle_ref, t.driver_name, COALESCE(t.parcel_kind,'TRANSFER'), t.status,
       t.printed_at, t.departed_at, t.delivered_at, t.cancelled_at, COALESCE(t.cancel_reason,''),
       t.created_by, t.operator_label, COALESCE(t.session_id::text, ''),
       t.notes, t.idempotency_key, t.created_at, t.updated_at,
       COALESCE(t.seal_number,''), COALESCE(t.seal_status,''), t.seal_verified_at,
       COALESCE(vu.idp_sub, COALESCE(t.seal_verified_by::text, '')), COALESCE(t.seal_notes,'')
FROM transport_sheets t
JOIN branches fb ON fb.id = t.from_branch_id
JOIN branches tb ON tb.id = t.to_branch_id
LEFT JOIN users vu ON vu.id = t.seal_verified_by
WHERE t.id = $1::uuid`, sheetID)
		sheet, err = scanTransportSheet(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		sections, err := loadTransportSections(ctx, tx, sheet.ID)
		if err != nil {
			return err
		}
		sheet.Sections = sections
		return nil
	})
	if err != nil {
		return domain.TransportSheet{}, err
	}
	return sheet, nil
}

func (s *Postgres) MarkTransportSheetPrinted(ctx context.Context, orgRef, sheetID, actor string) (domain.TransportSheet, error) {
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		if _, err := resolveOrgID(ctx, tx, orgRef); err != nil {
			return err
		}
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM transport_sheets WHERE id = $1::uuid FOR UPDATE`, sheetID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if status == domain.TransportStatusCancelled {
			return errors.New("sheet cancelled")
		}
		now := time.Now().UTC()
		next := domain.TransportStatusPrinted
		if status == domain.TransportStatusInTransit || status == domain.TransportStatusDelivered {
			next = status
		}
		_, err = tx.Exec(ctx, `
UPDATE transport_sheets
SET status = $1, printed_at = COALESCE(printed_at, $2), updated_at = $2
WHERE id = $3::uuid`, next, now, sheetID)
		_ = actor
		return err
	})
	if err != nil {
		return domain.TransportSheet{}, err
	}
	return s.GetTransportSheet(ctx, orgRef, sheetID)
}

type transportScanner interface {
	Scan(dest ...any) error
}

func scanTransportSheet(row transportScanner) (domain.TransportSheet, error) {
	var t domain.TransportSheet
	var printed, departed, delivered, cancelled, sealVerified *time.Time
	err := row.Scan(
		&t.ID, &t.OrgID, &t.SheetNumber,
		&t.FromBranchID, &t.ToBranchID,
		&t.CarrierName, &t.VehicleRef, &t.DriverName, &t.ParcelKind, &t.Status,
		&printed, &departed, &delivered, &cancelled, &t.CancelReason,
		&t.CreatedBy, &t.OperatorLabel, &t.SessionID,
		&t.Notes, &t.IdempotencyKey, &t.CreatedAt, &t.UpdatedAt,
		&t.SealNumber, &t.SealStatus, &sealVerified, &t.SealVerifiedBy, &t.SealNotes,
	)
	if err != nil {
		return domain.TransportSheet{}, err
	}
	t.PrintedAt = printed
	t.DepartedAt = departed
	t.DeliveredAt = delivered
	t.CancelledAt = cancelled
	t.SealVerifiedAt = sealVerified
	t.ParcelKindLabel = domain.ParcelKindLabelES(t.ParcelKind)
	return t, nil
}

func loadTransportSections(ctx context.Context, tx pgx.Tx, sheetID string) ([]domain.TransportSheetSection, error) {
	rows, err := tx.Query(ctx, `
SELECT
  sec.id::text,
  sec.department_code,
  sec.department_name,
  sec.notes,
  sec.sort_order,
  COALESCE((
    SELECT json_agg(json_build_object(
      'id', s.id::text,
      'slip_number', s.slip_number,
      'container_type', s.container_type,
      'description', s.description,
      'status', s.status
    ) ORDER BY s.slip_number)
    FROM transport_sheet_slip_links l
    JOIN shipping_slips s ON s.id = l.shipping_slip_id
    WHERE l.section_id = sec.id
  ), '[]'::json)
FROM transport_sheet_sections sec
WHERE sec.sheet_id = $1::uuid
ORDER BY sec.sort_order`, sheetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.TransportSheetSection
	for rows.Next() {
		var sec domain.TransportSheetSection
		var slipsRaw []byte
		if err := rows.Scan(&sec.ID, &sec.DepartmentCode, &sec.DepartmentName, &sec.Notes, &sec.SortOrder, &slipsRaw); err != nil {
			return nil, err
		}
		var slips []domain.TransportLinkedSlip
		if err := json.Unmarshal(slipsRaw, &slips); err != nil {
			return nil, err
		}
		for i := range slips {
			slips[i].ContainerLabel = domain.ContainerTypeLabelES(slips[i].ContainerType)
		}
		sec.Slips = slips
		out = append(out, sec)
	}
	return out, rows.Err()
}
