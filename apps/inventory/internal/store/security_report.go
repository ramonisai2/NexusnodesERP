package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/secure"
)

func (s *Postgres) ListSecurityLogistics(ctx context.Context, filter domain.SecurityLogisticsFilter) ([]domain.SecurityLogisticsEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	var out []domain.SecurityLogisticsEvent
	err := db.WithOrgTx(ctx, s.pool, filter.OrgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}

		kind := strings.ToUpper(strings.TrimSpace(filter.Kind))
		branch := strings.TrimSpace(filter.BranchCode)

		q := `
SELECT * FROM (
  SELECT 'TRANSPORT'::text AS kind, t.id::text AS id, t.sheet_number AS folio,
         fb.code AS branch_from, tb.code AS branch_to, ''::text AS branch_id,
         t.status AS status, t.carrier_name AS carrier_name, t.vehicle_ref AS vehicle_ref,
         t.driver_name AS driver_name, ''::text AS dock_door,
         COALESCE(t.seal_number,'') AS seal_number, COALESCE(t.seal_status,'') AS seal_status,
         t.seal_verified_at AS seal_verified_at,
         COALESCE((
           SELECT string_agg(DISTINCT s.container_type, ', ' ORDER BY s.container_type)
           FROM transport_sheet_sections sec
           JOIN transport_sheet_slip_links l ON l.section_id = sec.id
           JOIN shipping_slips s ON s.id = l.shipping_slip_id
           WHERE sec.sheet_id = t.id
         ), '') AS container_types,
         COALESCE(t.departed_at, t.delivered_at, t.printed_at, t.created_at) AS event_at,
         t.created_at AS created_at, COALESCE(t.seal_notes, t.notes, '') AS notes
  FROM transport_sheets t
  JOIN branches fb ON fb.id = t.from_branch_id
  JOIN branches tb ON tb.id = t.to_branch_id
  WHERE t.org_id = $1::uuid

  UNION ALL

  SELECT 'INBOUND', s.id::text, COALESCE(NULLIF(s.invoice_number,''), s.id::text),
         '' , '', b.code,
         s.status, s.carrier_name, s.vehicle_ref, s.driver_name, s.dock_door,
         COALESCE(s.seal_number,''), COALESCE(s.seal_status,''), s.seal_verified_at,
         (SELECT COUNT(*)::text || ' tarimas' FROM inbound_pallets p WHERE p.shipment_id = s.id),
         COALESCE(s.arrived_at, s.posted_at, s.created_at),
         s.created_at, COALESCE(s.seal_notes, s.notes, '')
  FROM inbound_shipments s
  JOIN branches b ON b.id = s.branch_id
  WHERE s.org_id = $1::uuid

  UNION ALL

  SELECT 'SLIP', sl.id::text, sl.slip_number,
         fb.code, tb.code, '',
         sl.status, '', '', '', '',
         '', '', NULL,
         sl.container_type,
         COALESCE(sl.shipped_at, sl.received_at, sl.printed_at, sl.created_at),
         sl.created_at, COALESCE(sl.notes, '')
  FROM shipping_slips sl
  JOIN branches fb ON fb.id = sl.from_branch_id
  JOIN branches tb ON tb.id = sl.to_branch_id
  WHERE sl.org_id = $1::uuid
) x WHERE 1=1`
		args := []any{orgID}
		n := 2
		if kind != "" {
			q += fmt.Sprintf(` AND x.kind = $%d`, n)
			args = append(args, kind)
			n++
		}
		if branch != "" {
			q += fmt.Sprintf(` AND (x.branch_from = $%d OR x.branch_to = $%d OR x.branch_id = $%d)`, n, n, n)
			args = append(args, branch)
			n++
		}
		if filter.SealOnly {
			q += ` AND x.seal_number <> ''`
		}
		q += fmt.Sprintf(` ORDER BY x.event_at DESC NULLS LAST, x.created_at DESC LIMIT %d`, limit)

		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.SecurityLogisticsEvent
			if err := rows.Scan(
				&e.Kind, &e.ID, &e.Folio,
				&e.BranchFrom, &e.BranchTo, &e.BranchID,
				&e.Status, &e.CarrierName, &e.VehicleRef, &e.DriverName, &e.DockDoor,
				&e.SealNumber, &e.SealStatus, &e.SealVerifiedAt,
				&e.ContainerTypes, &e.EventAt, &e.CreatedAt, &e.Notes,
			); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

func normalizeSealStatus(status string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(status))
	switch s {
	case domain.SealApplied, domain.SealVerified, domain.SealBroken, domain.SealMissing:
		return s, nil
	default:
		return "", fmt.Errorf("invalid seal_status: %s", status)
	}
}

func (s *Postgres) VerifyTransportSeal(ctx context.Context, orgRef, sheetID string, req domain.VerifySealRequest) (domain.TransportSheet, error) {
	status, err := normalizeSealStatus(req.SealStatus)
	if err != nil {
		return domain.TransportSheet{}, err
	}
	err = db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		actorID, _ := resolveUserID(ctx, tx, orgID, req.Actor)
		sealNumber := secure.PlainTextMax(req.SealNumber, 40)
		notes := secure.PlainTextMax(req.Notes, 400)
		now := time.Now().UTC()
		var existing string
		err = tx.QueryRow(ctx, `
SELECT id::text FROM transport_sheets WHERE org_id = $1::uuid AND id = $2::uuid`, orgID, sheetID).Scan(&existing)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
UPDATE transport_sheets
SET seal_number = CASE WHEN $2 <> '' THEN $2 ELSE seal_number END,
    seal_status = $3,
    seal_verified_at = $4,
    seal_verified_by = NULLIF($5,'')::uuid,
    seal_notes = $6,
    updated_at = $4
WHERE id = $1::uuid`, sheetID, sealNumber, status, now, actorID, notes)
		return err
	})
	if err != nil {
		return domain.TransportSheet{}, err
	}
	return s.GetTransportSheet(ctx, orgRef, sheetID)
}

func (s *Postgres) VerifyInboundSeal(ctx context.Context, orgRef, shipmentID string, req domain.VerifySealRequest) (domain.InboundShipment, error) {
	status, err := normalizeSealStatus(req.SealStatus)
	if err != nil {
		return domain.InboundShipment{}, err
	}
	err = db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		actorID, _ := resolveUserID(ctx, tx, orgID, req.Actor)
		sealNumber := secure.PlainTextMax(req.SealNumber, 40)
		notes := secure.PlainTextMax(req.Notes, 400)
		now := time.Now().UTC()
		var existing string
		err = tx.QueryRow(ctx, `
SELECT id::text FROM inbound_shipments WHERE org_id = $1::uuid AND id = $2::uuid`, orgID, shipmentID).Scan(&existing)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
UPDATE inbound_shipments
SET seal_number = CASE WHEN $2 <> '' THEN $2 ELSE seal_number END,
    seal_status = $3,
    seal_verified_at = $4,
    seal_verified_by = NULLIF($5,'')::uuid,
    seal_notes = $6
WHERE id = $1::uuid`, shipmentID, sealNumber, status, now, actorID, notes)
		return err
	})
	if err != nil {
		return domain.InboundShipment{}, err
	}
	return s.GetInboundShipment(ctx, orgRef, shipmentID)
}
