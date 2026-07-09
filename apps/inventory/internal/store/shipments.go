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
	"github.com/ramonisai2/NexusnodesERP/packages/go/secure"
)

func (s *Postgres) CreateInboundShipment(ctx context.Context, req domain.CreateInboundShipmentRequest) (domain.InboundShipment, error) {
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.IdempotencyKey == "" {
		return domain.InboundShipment{}, errors.New("idempotency_key required")
	}
	if len(req.Pallets) == 0 {
		return domain.InboundShipment{}, errors.New("pallets required")
	}
	req.SupplierName = secure.PlainTextMax(req.SupplierName, 120)
	req.InvoiceNumber = secure.PlainTextMax(req.InvoiceNumber, 80)
	req.CarrierName = secure.PlainTextMax(req.CarrierName, 80)
	req.VehicleRef = secure.PlainTextMax(req.VehicleRef, 40)
	req.DriverName = secure.PlainTextMax(req.DriverName, 80)
	req.DockDoor = secure.PlainTextMax(req.DockDoor, 20)
	req.Notes = secure.PlainTextMax(req.Notes, 400)
	if req.SupplierName == "" || req.InvoiceNumber == "" {
		return domain.InboundShipment{}, errors.New("supplier_name and invoice_number required")
	}

	var shipmentID string
	err := db.WithOrgTx(ctx, s.pool, req.OrgID, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, req.OrgID)
		if err != nil {
			return err
		}
		var existing string
		err = tx.QueryRow(ctx, `
SELECT id::text FROM inbound_shipments WHERE org_id = $1::uuid AND idempotency_key = $2`,
			orgID, req.IdempotencyKey).Scan(&existing)
		if err == nil {
			shipmentID = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		branchID, err := resolveBranchID(ctx, tx, orgID, req.BranchID)
		if err != nil {
			return err
		}
		whCode := strings.TrimSpace(req.WarehouseID)
		if whCode == "" {
			_ = tx.QueryRow(ctx, `
SELECT code FROM warehouses
WHERE branch_id = $1::uuid
ORDER BY CASE warehouse_kind WHEN 'CEDI' THEN 0 WHEN 'ARRIVAL' THEN 1 ELSE 2 END, code
LIMIT 1`, branchID).Scan(&whCode)
		}
		warehouseID, err := resolveWarehouseID(ctx, tx, branchID, whCode)
		if err != nil {
			return err
		}

		var invoiceDate any
		if strings.TrimSpace(req.InvoiceDate) != "" {
			d, err := time.Parse("2006-01-02", strings.TrimSpace(req.InvoiceDate))
			if err != nil {
				return errors.New("invalid invoice_date")
			}
			invoiceDate = d
		}
		createdBy, _ := resolveUserID(ctx, tx, orgID, req.CreatedBy)
		expected := req.ExpectedPallets
		if expected <= 0 {
			expected = len(req.Pallets)
		}

		id := uuid.New()
		shipmentID = id.String()
		now := time.Now().UTC()
		sealNumber := secure.PlainTextMax(req.SealNumber, 40)
		sealStatus := ""
		if sealNumber != "" {
			sealStatus = domain.SealApplied
		}
		_, err = tx.Exec(ctx, `
INSERT INTO inbound_shipments (
  id, org_id, branch_id, warehouse_id, supplier_name, invoice_number, invoice_date,
  carrier_name, vehicle_ref, driver_name, dock_door, expected_pallets, notes,
  status, arrived_at, created_by, operator_label, session_id, idempotency_key, created_at,
  seal_number, seal_status
) VALUES (
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7,
  $8, $9, $10, $11, $12, $13,
  'RECEIVING', $14, $15, $16, $17, $18, $14,
  $19, $20
)`, id, orgID, branchID, warehouseID, req.SupplierName, req.InvoiceNumber, invoiceDate,
			req.CarrierName, req.VehicleRef, req.DriverName, req.DockDoor, expected, req.Notes,
			now, createdBy, req.OperatorLabel, req.SessionID, req.IdempotencyKey, sealNumber, sealStatus)
		if err != nil {
			return err
		}

		for i, p := range req.Pallets {
			if len(p.Boxes) == 0 {
				return fmt.Errorf("pallet %d has no boxes", i+1)
			}
			palletNo := p.PalletNo
			if palletNo <= 0 {
				palletNo = i + 1
			}
			palletID := uuid.New()
			code := fmt.Sprintf("TAR-%s-%02d", strings.ToUpper(strings.ReplaceAll(req.BranchID, "_", "")), palletNo)
			label := secure.PlainTextMax(p.Label, 80)
			if label == "" {
				label = fmt.Sprintf("Tarima %d", palletNo)
			}
			_, err = tx.Exec(ctx, `
INSERT INTO inbound_pallets (id, shipment_id, pallet_no, pallet_code, label, status, notes)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'COUNTED', $6)`,
				palletID, id, palletNo, code, label, secure.PlainTextMax(p.Notes, 200))
			if err != nil {
				return err
			}
			for j, b := range p.Boxes {
				sku := strings.TrimSpace(b.SKU)
				if sku == "" {
					return fmt.Errorf("pallet %d box %d missing sku", palletNo, j+1)
				}
				boxes := b.BoxesCount
				if boxes <= 0 {
					boxes = 1
				}
				upb := b.UnitsPerBox
				if upb <= 0 {
					upb = 1
				}
				qty := float64(boxes) * upb
				skuID, err := resolveSKUID(ctx, tx, sku)
				if err != nil {
					return fmt.Errorf("sku not found: %s", sku)
				}
				var skuCode, desc string
				_ = tx.QueryRow(ctx, `
SELECT ps.sku, COALESCE(p.name, ps.sku)
FROM product_skus ps JOIN products p ON p.id = ps.product_id
WHERE ps.id = $1::uuid`, skuID).Scan(&skuCode, &desc)
				if strings.TrimSpace(b.Description) != "" {
					desc = secure.PlainTextMax(b.Description, 160)
				}
				boxCode := secure.PlainTextMax(b.BoxCode, 40)
				if boxCode == "" {
					boxCode = fmt.Sprintf("%s-C%02d", code, j+1)
				}
				_, err = tx.Exec(ctx, `
INSERT INTO inbound_boxes (
  id, pallet_id, box_no, box_code, sku_id, sku_code, description,
  boxes_count, units_per_box, quantity, unit_cost
) VALUES (
  $1::uuid, $2::uuid, $3, $4, $5::uuid, $6, $7, $8, $9, $10, $11
)`, uuid.New(), palletID, j+1, boxCode, skuID, skuCode, desc, boxes, upb, qty, b.UnitCost)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return domain.InboundShipment{}, err
	}
	return s.GetInboundShipment(ctx, req.OrgID, shipmentID)
}

func (s *Postgres) GetInboundShipment(ctx context.Context, orgRef, shipmentID string) (domain.InboundShipment, error) {
	var out domain.InboundShipment
	err := db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		var invDate *time.Time
		var receiptID *string
		err = tx.QueryRow(ctx, `
SELECT s.id::text, s.org_id::text, b.code, w.code, COALESCE(w.warehouse_kind,''),
       s.receipt_id::text, s.supplier_name, s.invoice_number, s.invoice_date,
       s.carrier_name, s.vehicle_ref, s.driver_name, s.dock_door, s.expected_pallets,
       s.notes, s.status, s.arrived_at, s.posted_at, s.created_at,
       COALESCE(s.seal_number,''), COALESCE(s.seal_status,''), s.seal_verified_at,
       COALESCE(vu.idp_sub, COALESCE(s.seal_verified_by::text, '')), COALESCE(s.seal_notes,'')
FROM inbound_shipments s
JOIN branches b ON b.id = s.branch_id
JOIN warehouses w ON w.id = s.warehouse_id
LEFT JOIN users vu ON vu.id = s.seal_verified_by
WHERE s.org_id = $1::uuid AND s.id = $2::uuid`, orgID, shipmentID).Scan(
			&out.ID, &out.OrgID, &out.BranchID, &out.WarehouseID, &out.WarehouseKind,
			&receiptID, &out.SupplierName, &out.InvoiceNumber, &invDate,
			&out.CarrierName, &out.VehicleRef, &out.DriverName, &out.DockDoor, &out.ExpectedPallets,
			&out.Notes, &out.Status, &out.ArrivedAt, &out.PostedAt, &out.CreatedAt,
			&out.SealNumber, &out.SealStatus, &out.SealVerifiedAt, &out.SealVerifiedBy, &out.SealNotes,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if receiptID != nil {
			out.ReceiptID = *receiptID
		}
		if invDate != nil {
			d := invDate.Format("2006-01-02")
			out.InvoiceDate = &d
		}

		prows, err := tx.Query(ctx, `
SELECT id::text, pallet_no, pallet_code, label, status, notes
FROM inbound_pallets WHERE shipment_id = $1::uuid ORDER BY pallet_no`, shipmentID)
		if err != nil {
			return err
		}
		var pallets []domain.InboundPallet
		for prows.Next() {
			var p domain.InboundPallet
			if err := prows.Scan(&p.ID, &p.PalletNo, &p.PalletCode, &p.Label, &p.Status, &p.Notes); err != nil {
				prows.Close()
				return err
			}
			pallets = append(pallets, p)
		}
		prows.Close()
		if err := prows.Err(); err != nil {
			return err
		}

		skuAgg := map[string]*domain.ReceiptLine{}
		for i := range pallets {
			p := &pallets[i]
			brows, err := tx.Query(ctx, `
SELECT id::text, box_no, box_code, sku_code, description, boxes_count, units_per_box::float8,
       quantity::float8, unit_cost::float8
FROM inbound_boxes WHERE pallet_id = $1::uuid ORDER BY box_no`, p.ID)
			if err != nil {
				return err
			}
			for brows.Next() {
				var b domain.InboundBox
				var cost *float64
				if err := brows.Scan(&b.ID, &b.BoxNo, &b.BoxCode, &b.SKU, &b.Description, &b.BoxesCount, &b.UnitsPerBox, &b.Quantity, &cost); err != nil {
					brows.Close()
					return err
				}
				b.UnitCost = cost
				p.Boxes = append(p.Boxes, b)
				p.BoxCount += b.BoxesCount
				p.UnitTotal += b.Quantity
				out.BoxCount += b.BoxesCount
				out.UnitTotal += b.Quantity
				if agg, ok := skuAgg[b.SKU]; ok {
					agg.Quantity += b.Quantity
				} else {
					skuAgg[b.SKU] = &domain.ReceiptLine{
						SKU: b.SKU, Quantity: b.Quantity, UnitCost: cost, LabelDescription: b.Description,
					}
				}
			}
			brows.Close()
			if err := brows.Err(); err != nil {
				return err
			}
		}
		out.Pallets = pallets
		out.PalletCount = len(out.Pallets)
		i := 0
		for _, line := range skuAgg {
			i++
			line.SortOrder = i
			out.SKUSummary = append(out.SKUSummary, *line)
		}
		return nil
	})
	if err != nil {
		return domain.InboundShipment{}, err
	}
	if out.ReceiptID != "" {
		rec, err := s.GetReceipt(ctx, orgRef, out.ReceiptID)
		if err == nil {
			out.Receipt = &rec
		}
	}
	return out, nil
}

func (s *Postgres) ListInboundShipments(ctx context.Context, filter domain.InboundShipmentFilter) ([]domain.InboundShipment, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var out []domain.InboundShipment
	err := db.WithOrgTx(ctx, s.pool, filter.OrgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}
		q := `
SELECT s.id::text, b.code, w.code, s.supplier_name, s.invoice_number, s.status,
       s.expected_pallets, s.vehicle_ref, s.carrier_name, s.arrived_at, s.posted_at, s.created_at,
       (SELECT COUNT(*) FROM inbound_pallets p WHERE p.shipment_id = s.id)::int,
       COALESCE(s.seal_number,''), COALESCE(s.seal_status,'')
FROM inbound_shipments s
JOIN branches b ON b.id = s.branch_id
JOIN warehouses w ON w.id = s.warehouse_id
WHERE s.org_id = $1::uuid`
		args := []any{orgID}
		n := 2
		if filter.BranchCode != "" {
			q += fmt.Sprintf(` AND b.code = $%d`, n)
			args = append(args, filter.BranchCode)
			n++
		}
		if filter.Status != "" {
			q += fmt.Sprintf(` AND s.status = $%d`, n)
			args = append(args, filter.Status)
			n++
		}
		q += fmt.Sprintf(` ORDER BY s.created_at DESC LIMIT %d`, limit)
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sh domain.InboundShipment
			if err := rows.Scan(&sh.ID, &sh.BranchID, &sh.WarehouseID, &sh.SupplierName, &sh.InvoiceNumber, &sh.Status,
				&sh.ExpectedPallets, &sh.VehicleRef, &sh.CarrierName, &sh.ArrivedAt, &sh.PostedAt, &sh.CreatedAt, &sh.PalletCount,
				&sh.SealNumber, &sh.SealStatus); err != nil {
				return err
			}
			out = append(out, sh)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Postgres) PostInboundShipment(ctx context.Context, orgRef, shipmentID, postedBy string) (domain.InboundShipment, error) {
	ship, err := s.GetInboundShipment(ctx, orgRef, shipmentID)
	if err != nil {
		return domain.InboundShipment{}, err
	}
	if ship.Status == domain.ShipmentPosted && ship.ReceiptID != "" {
		return s.GetInboundShipment(ctx, orgRef, shipmentID)
	}
	if len(ship.SKUSummary) == 0 {
		return domain.InboundShipment{}, errors.New("shipment has no units")
	}

	printLabels := true
	lines := make([]domain.ReceiptLineInput, 0, len(ship.SKUSummary))
	for _, l := range ship.SKUSummary {
		lines = append(lines, domain.ReceiptLineInput{
			SKU: l.SKU, Quantity: l.Quantity, UnitCost: l.UnitCost, LabelDescription: l.LabelDescription, LabelPrice: nil,
		})
	}
	invDate := ""
	if ship.InvoiceDate != nil {
		invDate = *ship.InvoiceDate
	}
	notes := ship.Notes
	if ship.VehicleRef != "" || ship.CarrierName != "" {
		extra := strings.TrimSpace(fmt.Sprintf("Camión %s %s · %d tarimas", ship.CarrierName, ship.VehicleRef, ship.PalletCount))
		if notes != "" {
			notes = notes + " | " + extra
		} else {
			notes = extra
		}
	}
	rec, err := s.CreateReceipt(ctx, domain.CreateReceiptRequest{
		OrgID:          orgRef,
		BranchID:       ship.BranchID,
		WarehouseID:    ship.WarehouseID,
		SupplierName:   ship.SupplierName,
		InvoiceNumber:  ship.InvoiceNumber,
		InvoiceDate:    invDate,
		Notes:          notes,
		PrintLabels:    &printLabels,
		IdempotencyKey: "ship-" + shipmentID,
		CreatedBy:      postedBy,
		Lines:          lines,
	})
	if err != nil {
		return domain.InboundShipment{}, err
	}
	posted, err := s.PostReceipt(ctx, orgRef, rec.ID, postedBy)
	if err != nil {
		return domain.InboundShipment{}, err
	}

	err = db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
UPDATE inbound_shipments
SET status = 'POSTED', posted_at = now(), receipt_id = $2::uuid
WHERE id = $1::uuid`, shipmentID, posted.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE inbound_pallets SET status = 'POSTED' WHERE shipment_id = $1::uuid`, shipmentID)
		if err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `UPDATE inbound_receipts SET shipment_id = $1::uuid WHERE id = $2::uuid`, shipmentID, posted.ID)
		_, _ = tx.Exec(ctx, `
INSERT INTO outbox (event_type, payload) VALUES ('InboundShipmentPosted', jsonb_build_object(
  'shipment_id', $1::text,
  'receipt_id', $2::text,
  'pallet_count', $3::int,
  'unit_total', $4::float8,
  'warehouse_id', $5::text
))`, shipmentID, posted.ID, ship.PalletCount, ship.UnitTotal, ship.WarehouseID)
		return nil
	})
	if err != nil {
		return domain.InboundShipment{}, err
	}
	return s.GetInboundShipment(ctx, orgRef, shipmentID)
}
