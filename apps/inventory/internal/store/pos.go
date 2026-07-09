package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/secure"
)

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func splitTaxInclusive(gross, rate float64) (base, tax float64) {
	if rate <= 0 {
		return round2(gross), 0
	}
	base = round2(gross / (1 + rate))
	tax = round2(gross - base)
	return base, tax
}

func paymentLabel(method string) string {
	switch strings.ToUpper(method) {
	case domain.PaymentCash:
		return "Efectivo"
	case domain.PaymentCard:
		return "Tarjeta"
	case domain.PaymentTransfer:
		return "Transferencia"
	default:
		return "Otro"
	}
}

func invoiceStatusLabel(status string) string {
	switch status {
	case "REQUESTED":
		return "Solicitada (pendiente de timbrar)"
	case "STAMPED":
		return "Timbrada"
	case "CANCELLED":
		return "Cancelada"
	case "ERROR":
		return "Error"
	default:
		return status
	}
}

func (s *Postgres) GetFiscalSettings(ctx context.Context, orgRef, branchCode string) (domain.BranchFiscalSettings, error) {
	var out domain.BranchFiscalSettings
	err := db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		branchID, err := resolveBranchID(ctx, tx, orgID, branchCode)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
SELECT b.code, fs.org_id::text, fs.legal_name, fs.trade_name, fs.rfc, fs.tax_regime, fs.postal_code,
       fs.prices_include_tax, fs.default_tax_rate::float8, fs.ticket_series, fs.next_ticket_folio,
       fs.invoice_series, fs.receipt_footer
FROM branch_fiscal_settings fs
JOIN branches b ON b.id = fs.branch_id
WHERE fs.branch_id = $1::uuid`, branchID).Scan(
			&out.BranchID, &out.OrgID, &out.LegalName, &out.TradeName, &out.RFC, &out.TaxRegime, &out.PostalCode,
			&out.PricesIncludeTax, &out.DefaultTaxRate, &out.TicketSeries, &out.NextTicketFolio,
			&out.InvoiceSeries, &out.ReceiptFooter,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			// Sensible MX defaults if row missing.
			var trade string
			_ = tx.QueryRow(ctx, `SELECT name FROM branches WHERE id = $1::uuid`, branchID).Scan(&trade)
			out = domain.BranchFiscalSettings{
				BranchID:         branchCode,
				OrgID:            orgID,
				LegalName:        trade,
				TradeName:        trade,
				RFC:              "XAXX010101000",
				TaxRegime:        "616",
				PostalCode:       "00000",
				PricesIncludeTax: true,
				DefaultTaxRate:   0.16,
				TicketSeries:     "T",
				NextTicketFolio:  1,
				InvoiceSeries:    "F",
				ReceiptFooter:    "Gracias por su compra.",
			}
			return nil
		}
		return err
	})
	return out, err
}

func (s *Postgres) UpsertFiscalSettings(ctx context.Context, req domain.UpsertFiscalSettingsRequest) (domain.BranchFiscalSettings, error) {
	req.LegalName = secure.PlainTextMax(req.LegalName, 160)
	req.TradeName = secure.PlainTextMax(req.TradeName, 120)
	req.RFC = strings.ToUpper(secure.PlainTextMax(req.RFC, 13))
	req.TaxRegime = secure.PlainTextMax(req.TaxRegime, 10)
	req.PostalCode = secure.PlainTextMax(req.PostalCode, 10)
	req.TicketSeries = secure.PlainTextMax(req.TicketSeries, 8)
	req.InvoiceSeries = secure.PlainTextMax(req.InvoiceSeries, 8)
	req.ReceiptFooter = secure.PlainTextMax(req.ReceiptFooter, 280)
	if msg := secure.RejectIfInjection("legal_name", req.LegalName); msg != "" {
		return domain.BranchFiscalSettings{}, errors.New(msg)
	}

	err := db.WithOrgTx(ctx, s.pool, req.OrgID, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, req.OrgID)
		if err != nil {
			return err
		}
		branchID, err := resolveBranchID(ctx, tx, orgID, req.BranchID)
		if err != nil {
			return err
		}
		include := true
		if req.PricesIncludeTax != nil {
			include = *req.PricesIncludeTax
		}
		rate := 0.16
		if req.DefaultTaxRate != nil && *req.DefaultTaxRate >= 0 {
			rate = *req.DefaultTaxRate
		}
		if req.TicketSeries == "" {
			req.TicketSeries = "T"
		}
		if req.InvoiceSeries == "" {
			req.InvoiceSeries = "F"
		}
		_, err = tx.Exec(ctx, `
INSERT INTO branch_fiscal_settings (
  branch_id, org_id, legal_name, trade_name, rfc, tax_regime, postal_code,
  prices_include_tax, default_tax_rate, ticket_series, invoice_series, receipt_footer, updated_at
) VALUES (
  $1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now()
)
ON CONFLICT (branch_id) DO UPDATE SET
  legal_name = EXCLUDED.legal_name,
  trade_name = EXCLUDED.trade_name,
  rfc = EXCLUDED.rfc,
  tax_regime = EXCLUDED.tax_regime,
  postal_code = EXCLUDED.postal_code,
  prices_include_tax = EXCLUDED.prices_include_tax,
  default_tax_rate = EXCLUDED.default_tax_rate,
  ticket_series = EXCLUDED.ticket_series,
  invoice_series = EXCLUDED.invoice_series,
  receipt_footer = EXCLUDED.receipt_footer,
  updated_at = now()`,
			branchID, orgID, req.LegalName, req.TradeName, req.RFC, req.TaxRegime, req.PostalCode,
			include, rate, req.TicketSeries, req.InvoiceSeries, req.ReceiptFooter)
		return err
	})
	if err != nil {
		return domain.BranchFiscalSettings{}, err
	}
	return s.GetFiscalSettings(ctx, req.OrgID, req.BranchID)
}

func (s *Postgres) CompleteSale(ctx context.Context, req domain.CompleteSaleRequest) (domain.POSSale, error) {
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.IdempotencyKey == "" {
		return domain.POSSale{}, errors.New("idempotency_key required")
	}
	if len(req.Lines) == 0 {
		return domain.POSSale{}, errors.New("lines required")
	}
	if len(req.Payments) == 0 {
		return domain.POSSale{}, errors.New("payments required")
	}
	req.CustomerName = secure.PlainTextMax(req.CustomerName, 120)
	req.Notes = secure.PlainTextMax(req.Notes, 280)
	req.InvoiceRFC = strings.ToUpper(secure.PlainTextMax(req.InvoiceRFC, 13))
	req.InvoiceName = secure.PlainTextMax(req.InvoiceName, 160)

	var saleID string
	err := db.WithOrgTx(ctx, s.pool, req.OrgID, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, req.OrgID)
		if err != nil {
			return err
		}
		// Idempotent replay
		var existingID string
		err = tx.QueryRow(ctx, `
SELECT id::text FROM pos_sales WHERE org_id = $1::uuid AND idempotency_key = $2`, orgID, req.IdempotencyKey).Scan(&existingID)
		if err == nil {
			saleID = existingID
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		branchID, err := resolveBranchID(ctx, tx, orgID, req.BranchID)
		if err != nil {
			return err
		}
		branchCode := req.BranchID
		_ = tx.QueryRow(ctx, `SELECT code FROM branches WHERE id = $1::uuid`, branchID).Scan(&branchCode)

		whCode := strings.TrimSpace(req.WarehouseID)
		if whCode == "" {
			_ = tx.QueryRow(ctx, `
SELECT code FROM warehouses
WHERE branch_id = $1::uuid
ORDER BY CASE warehouse_kind WHEN 'STORE' THEN 0 WHEN 'ARRIVAL' THEN 1 ELSE 2 END, code
LIMIT 1`, branchID).Scan(&whCode)
		}
		if whCode == "" {
			return errors.New("warehouse required")
		}
		warehouseID, err := resolveWarehouseID(ctx, tx, branchID, whCode)
		if err != nil {
			return err
		}

		fiscal, err := loadFiscalInTx(ctx, tx, orgID, branchID, branchCode)
		if err != nil {
			return err
		}
		rate := fiscal.DefaultTaxRate
		if rate <= 0 {
			rate = 0.16
		}

		type builtLine struct {
			skuID, skuCode, desc string
			qty, unit, lineTotal, base, tax float64
		}
		var built []builtLine
		var subtotal, taxTotal, grand float64
		for i, line := range req.Lines {
			if line.Quantity <= 0 || strings.TrimSpace(line.SKU) == "" {
				return fmt.Errorf("invalid line %d", i+1)
			}
			skuID, err := resolveSKUID(ctx, tx, line.SKU)
			if err != nil {
				return fmt.Errorf("sku not found: %s", line.SKU)
			}
			var skuCode, desc string
			_ = tx.QueryRow(ctx, `
SELECT ps.sku, COALESCE(NULLIF(ssl.public_description, ''), p.name, ps.sku)
FROM product_skus ps
JOIN products p ON p.id = ps.product_id
LEFT JOIN store_sku_labels ssl ON ssl.sku_id = ps.id AND ssl.branch_id = $2::uuid
WHERE ps.id = $1::uuid`, skuID, branchID).Scan(&skuCode, &desc)

			var commonP, specialP, finalP *float64
			var priceMode string
			var c, sp, f *float64
			err = tx.QueryRow(ctx, `
SELECT common_price, special_price, final_price, COALESCE(price_mode, 'COMMON')
FROM store_sku_labels
WHERE branch_id = $1::uuid AND sku_id = $2::uuid`, branchID, skuID).Scan(&c, &sp, &f, &priceMode)
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("no price for sku %s", skuCode)
			}
			if err != nil {
				return err
			}
			commonP, specialP, finalP = c, sp, f
			lbl := domain.StoreLabel{CommonPrice: commonP, SpecialPrice: specialP, FinalPrice: finalP, PriceMode: priceMode}
			eff := lbl.ResolveEffectivePrice()
			if eff == nil {
				return fmt.Errorf("no price for sku %s", skuCode)
			}
			unit := round2(*eff)
			lineTotal := round2(unit * line.Quantity)
			var base, tax float64
			if fiscal.PricesIncludeTax {
				base, tax = splitTaxInclusive(lineTotal, rate)
			} else {
				base = lineTotal
				tax = round2(lineTotal * rate)
				lineTotal = round2(base + tax)
			}
			built = append(built, builtLine{skuID, skuCode, desc, line.Quantity, unit, lineTotal, base, tax})
			subtotal += base
			taxTotal += tax
			grand += lineTotal
		}
		subtotal = round2(subtotal)
		taxTotal = round2(taxTotal)
		grand = round2(grand)

		var paySum float64
		for _, p := range req.Payments {
			method := strings.ToUpper(strings.TrimSpace(p.Method))
			if method != domain.PaymentCash && method != domain.PaymentCard && method != domain.PaymentTransfer && method != domain.PaymentOther {
				return fmt.Errorf("invalid payment method %s", p.Method)
			}
			if p.Amount < 0 {
				return errors.New("invalid payment amount")
			}
			paySum += p.Amount
		}
		paySum = round2(paySum)
		if paySum+0.009 < grand {
			return fmt.Errorf("payments %.2f cover less than total %.2f", paySum, grand)
		}

		var folio int64
		err = tx.QueryRow(ctx, `
UPDATE branch_fiscal_settings
SET next_ticket_folio = next_ticket_folio + 1, updated_at = now()
WHERE branch_id = $1::uuid
RETURNING next_ticket_folio - 1`, branchID).Scan(&folio)
		if errors.Is(err, pgx.ErrNoRows) {
			_, _ = tx.Exec(ctx, `
INSERT INTO branch_fiscal_settings (branch_id, org_id, legal_name, trade_name, prices_include_tax, default_tax_rate)
VALUES ($1::uuid, $2::uuid, $3, $3, TRUE, 0.16)`, branchID, orgID, fiscal.TradeName)
			err = tx.QueryRow(ctx, `
UPDATE branch_fiscal_settings
SET next_ticket_folio = next_ticket_folio + 1
WHERE branch_id = $1::uuid
RETURNING next_ticket_folio - 1`, branchID).Scan(&folio)
		}
		if err != nil {
			return err
		}
		series := fiscal.TicketSeries
		if series == "" {
			series = "T"
		}
		ticket := fmt.Sprintf("%s-%s-%06d", series, strings.ToUpper(branchCode), folio)

		var customerID any
		if req.CustomerID != "" {
			if _, err := uuid.Parse(req.CustomerID); err == nil {
				customerID = req.CustomerID
			}
		}
		saleUUID := uuid.New()
		saleID = saleUUID.String()
		now := time.Now().UTC()
		_, err = tx.Exec(ctx, `
INSERT INTO pos_sales (
  id, org_id, branch_id, warehouse_id, ticket_number, status, currency,
  prices_include_tax, tax_rate, subtotal, tax_total, discount_total, grand_total,
  customer_id, customer_name, cashier_sub, operator_label, session_id, station_id,
  notes, request_invoice, idempotency_key, completed_at, created_at
) VALUES (
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, 'COMPLETED', 'MXN',
  $6, $7, $8, $9, 0, $10,
  $11::uuid, $12, $13, $14, $15, $16,
  $17, $18, $19, $20, $20
)`, saleUUID, orgID, branchID, warehouseID, ticket,
			fiscal.PricesIncludeTax, rate, subtotal, taxTotal, grand,
			customerID, req.CustomerName, req.CashierSub, req.OperatorLabel, req.SessionID, req.StationID,
			req.Notes, req.RequestInvoice, req.IdempotencyKey, now)
		if err != nil {
			return err
		}

		postedBy, _ := resolveUserID(ctx, tx, orgID, req.CashierSub)
		for i, bl := range built {
			movID := uuid.New()
			idem := fmt.Sprintf("%s-line-%d", req.IdempotencyKey, i+1)
			var balID string
			var onHand float64
			var version int
			err = tx.QueryRow(ctx, `
SELECT id::text, on_hand::float8, version
FROM stock_balances WHERE warehouse_id = $1::uuid AND sku_id = $2::uuid
FOR UPDATE`, warehouseID, bl.skuID).Scan(&balID, &onHand, &version)
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInsufficientStock
			}
			if err != nil {
				return err
			}
			next := onHand - bl.qty
			if next < -0.0001 {
				return fmt.Errorf("%w: %s", domain.ErrInsufficientStock, bl.skuCode)
			}
			ct, err := tx.Exec(ctx, `
UPDATE stock_balances SET on_hand = $1, version = version + 1, updated_at = now()
WHERE id = $2::uuid AND version = $3`, next, balID, version)
			if err != nil {
				return err
			}
			if ct.RowsAffected() == 0 {
				return domain.ErrConflict
			}
			var sess any
			if req.SessionID != "" {
				if _, err := uuid.Parse(req.SessionID); err == nil {
					sess = req.SessionID
				}
			}
			_, err = tx.Exec(ctx, `
INSERT INTO inventory_movements (
  id, org_id, branch_id, sku_id, warehouse_id, movement_type, quantity, status,
  posted_by, idempotency_key, created_at, operator_label, session_id, source_sale_id
) VALUES (
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, 'ISSUE', $6, 'POSTED',
  $7::uuid, $8, $9, $10, $11::uuid, $12::uuid
)`, movID, orgID, branchID, bl.skuID, warehouseID, bl.qty,
				postedBy, idem, now, req.OperatorLabel, sess, saleUUID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
INSERT INTO pos_sale_lines (
  id, sale_id, line_no, sku_id, sku_code, description, quantity, unit_price,
  line_total, tax_rate, tax_amount, base_amount, movement_id
) VALUES (
  $1::uuid, $2::uuid, $3, $4::uuid, $5, $6, $7, $8, $9, $10, $11, $12, $13::uuid
)`, uuid.New(), saleUUID, i+1, bl.skuID, bl.skuCode, bl.desc, bl.qty, bl.unit,
				bl.lineTotal, rate, bl.tax, bl.base, movID)
			if err != nil {
				return err
			}
		}

		for _, p := range req.Payments {
			method := strings.ToUpper(strings.TrimSpace(p.Method))
			change := 0.0
			var received any
			if p.ReceivedAmount != nil {
				received = *p.ReceivedAmount
				if method == domain.PaymentCash && *p.ReceivedAmount > p.Amount {
					change = round2(*p.ReceivedAmount - p.Amount)
				}
			}
			_, err = tx.Exec(ctx, `
INSERT INTO pos_payments (id, sale_id, method, amount, received_amount, change_amount, reference)
VALUES ($1::uuid, $2::uuid, $3, $4, $5::numeric, $6, $7)`,
				uuid.New(), saleUUID, method, p.Amount, received, change, secure.PlainTextMax(p.Reference, 80))
			if err != nil {
				return err
			}
		}

		if req.RequestInvoice {
			if err := insertInvoiceRequest(ctx, tx, orgID, saleUUID, fiscal, req, subtotal, taxTotal, grand); err != nil {
				return err
			}
		}

		_, _ = tx.Exec(ctx, `
INSERT INTO outbox (event_type, payload) VALUES ('SaleCompleted', jsonb_build_object(
  'sale_id', $1::text,
  'ticket_number', $2::text,
  'grand_total', $3::float8,
  'branch_id', $4::text,
  'org_id', $5::text
))`, saleUUID.String(), ticket, grand, branchCode, orgID)
		return nil
	})
	if err != nil {
		return domain.POSSale{}, err
	}
	return s.GetSale(ctx, req.OrgID, saleID)
}

func loadFiscalInTx(ctx context.Context, tx pgx.Tx, orgID, branchID, branchCode string) (domain.BranchFiscalSettings, error) {
	var out domain.BranchFiscalSettings
	err := tx.QueryRow(ctx, `
SELECT org_id::text, legal_name, trade_name, rfc, tax_regime, postal_code,
       prices_include_tax, default_tax_rate::float8, ticket_series, next_ticket_folio,
       invoice_series, receipt_footer
FROM branch_fiscal_settings WHERE branch_id = $1::uuid`, branchID).Scan(
		&out.OrgID, &out.LegalName, &out.TradeName, &out.RFC, &out.TaxRegime, &out.PostalCode,
		&out.PricesIncludeTax, &out.DefaultTaxRate, &out.TicketSeries, &out.NextTicketFolio,
		&out.InvoiceSeries, &out.ReceiptFooter,
	)
	out.BranchID = branchCode
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BranchFiscalSettings{
			BranchID: branchCode, OrgID: orgID, PricesIncludeTax: true, DefaultTaxRate: 0.16,
			TicketSeries: "T", InvoiceSeries: "F", ReceiptFooter: "Gracias por su compra.",
		}, nil
	}
	return out, err
}

func insertInvoiceRequest(ctx context.Context, tx pgx.Tx, orgID string, saleUUID uuid.UUID, fiscal domain.BranchFiscalSettings, req domain.CompleteSaleRequest, subtotal, taxTotal, grand float64) error {
	rfc := req.InvoiceRFC
	name := req.InvoiceName
	if rfc == "" || name == "" {
		return errors.New("invoice requires rfc and legal_name")
	}
	regime := req.InvoiceRegime
	if regime == "" {
		regime = "616"
	}
	postal := req.InvoicePostal
	if postal == "" {
		postal = fiscal.PostalCode
	}
	uso := req.InvoiceUsoCFDI
	if uso == "" {
		uso = "G03"
	}
	series := fiscal.InvoiceSeries
	if series == "" {
		series = "F"
	}
	var folio int64
	_ = tx.QueryRow(ctx, `
UPDATE branch_fiscal_settings
SET next_invoice_folio = next_invoice_folio + 1
WHERE branch_id = (SELECT branch_id FROM pos_sales WHERE id = $1::uuid)
RETURNING next_invoice_folio - 1`, saleUUID).Scan(&folio)
	_, err := tx.Exec(ctx, `
INSERT INTO fiscal_invoices (
  id, org_id, sale_id, status, series, folio, rfc_receiver, legal_name_receiver,
  tax_regime_receiver, postal_code_receiver, uso_cfdi, email_cfdi,
  subtotal, tax_total, grand_total, notes, requested_at
) VALUES (
  $1::uuid, $2::uuid, $3::uuid, 'REQUESTED', $4, $5, $6, $7, $8, $9, $10, $11,
  $12, $13, $14, $15, now()
)
ON CONFLICT (sale_id) DO NOTHING`,
		uuid.New(), orgID, saleUUID, series, folio, rfc, name, regime, postal, uso,
		secure.PlainTextMax(req.InvoiceEmail, 120), subtotal, taxTotal, grand,
		"Solicitud registrada. El timbrado CFDI con PAC se conecta en una fase posterior.")
	return err
}

func (s *Postgres) GetSale(ctx context.Context, orgRef, saleID string) (domain.POSSale, error) {
	var out domain.POSSale
	err := db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		var custID *string
		var completed *time.Time
		var whCode *string
		err = tx.QueryRow(ctx, `
SELECT s.id::text, s.org_id::text, b.code, COALESCE(w.code, ''), s.ticket_number, s.status, s.currency,
       s.prices_include_tax, s.tax_rate::float8, s.subtotal::float8, s.tax_total::float8,
       s.discount_total::float8, s.grand_total::float8, s.customer_id::text, s.customer_name,
       s.cashier_sub, s.operator_label, s.session_id, s.station_id, s.notes, s.request_invoice,
       s.completed_at, s.created_at
FROM pos_sales s
JOIN branches b ON b.id = s.branch_id
LEFT JOIN warehouses w ON w.id = s.warehouse_id
WHERE s.org_id = $1::uuid AND s.id = $2::uuid`, orgID, saleID).Scan(
			&out.ID, &out.OrgID, &out.BranchID, &out.WarehouseID, &out.TicketNumber, &out.Status, &out.Currency,
			&out.PricesIncludeTax, &out.TaxRate, &out.Subtotal, &out.TaxTotal, &out.DiscountTotal, &out.GrandTotal,
			&custID, &out.CustomerName, &out.CashierSub, &out.OperatorLabel, &out.SessionID, &out.StationID,
			&out.Notes, &out.RequestInvoice, &completed, &out.CreatedAt,
		)
		_ = whCode
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if custID != nil {
			out.CustomerID = *custID
		}
		out.CompletedAt = completed

		rows, err := tx.Query(ctx, `
SELECT id::text, line_no, sku_code, description, quantity::float8, unit_price::float8,
       line_total::float8, tax_rate::float8, tax_amount::float8, base_amount::float8,
       COALESCE(movement_id::text, '')
FROM pos_sale_lines WHERE sale_id = $1::uuid ORDER BY line_no`, saleID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ln domain.POSSaleLine
			if err := rows.Scan(&ln.ID, &ln.LineNo, &ln.SKU, &ln.Description, &ln.Quantity, &ln.UnitPrice,
				&ln.LineTotal, &ln.TaxRate, &ln.TaxAmount, &ln.BaseAmount, &ln.MovementID); err != nil {
				return err
			}
			out.Lines = append(out.Lines, ln)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		prows, err := tx.Query(ctx, `
SELECT id::text, method, amount::float8, received_amount::float8, change_amount::float8, reference
FROM pos_payments WHERE sale_id = $1::uuid ORDER BY created_at`, saleID)
		if err != nil {
			return err
		}
		defer prows.Close()
		for prows.Next() {
			var p domain.POSPayment
			var received *float64
			if err := prows.Scan(&p.ID, &p.Method, &p.Amount, &received, &p.ChangeAmount, &p.Reference); err != nil {
				return err
			}
			p.ReceivedAmount = received
			p.MethodLabel = paymentLabel(p.Method)
			out.Payments = append(out.Payments, p)
		}
		if err := prows.Err(); err != nil {
			return err
		}

		var inv domain.FiscalInvoice
		var folio *int64
		var stamped *time.Time
		err = tx.QueryRow(ctx, `
SELECT id::text, sale_id::text, status, series, folio, COALESCE(uuid, ''), rfc_receiver, legal_name_receiver,
       tax_regime_receiver, postal_code_receiver, uso_cfdi, email_cfdi,
       subtotal::float8, tax_total::float8, grand_total::float8, notes, requested_at, stamped_at
FROM fiscal_invoices WHERE sale_id = $1::uuid`, saleID).Scan(
			&inv.ID, &inv.SaleID, &inv.Status, &inv.Series, &folio, &inv.UUID, &inv.RFCReceiver, &inv.LegalNameReceiver,
			&inv.TaxRegimeReceiver, &inv.PostalCodeReceiver, &inv.UsoCFDI, &inv.EmailCFDI,
			&inv.Subtotal, &inv.TaxTotal, &inv.GrandTotal, &inv.Notes, &inv.RequestedAt, &stamped,
		)
		if err == nil {
			inv.Folio = folio
			inv.StampedAt = stamped
			inv.StatusLabel = invoiceStatusLabel(inv.Status)
			out.Invoice = &inv
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		fs2, err2 := s.GetFiscalSettings(ctx, orgRef, out.BranchID)
		if err2 == nil {
			out.Fiscal = &fs2
			out.ReceiptHint = fs2.ReceiptFooter
		}
		return nil
	})
	return out, err
}

func (s *Postgres) ListSales(ctx context.Context, filter domain.SaleFilter) ([]domain.POSSale, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var out []domain.POSSale
	err := db.WithOrgTx(ctx, s.pool, filter.OrgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}
		q := `
SELECT s.id::text, b.code, s.ticket_number, s.status, s.grand_total::float8, s.tax_total::float8,
       s.customer_name, s.operator_label, s.request_invoice, s.completed_at, s.created_at
FROM pos_sales s
JOIN branches b ON b.id = s.branch_id
WHERE s.org_id = $1::uuid`
		args := []any{orgID}
		if filter.BranchCode != "" {
			q += ` AND b.code = $2`
			args = append(args, filter.BranchCode)
			q += fmt.Sprintf(` ORDER BY s.completed_at DESC NULLS LAST, s.created_at DESC LIMIT %d`, limit)
		} else {
			q += fmt.Sprintf(` ORDER BY s.completed_at DESC NULLS LAST, s.created_at DESC LIMIT %d`, limit)
		}
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var srow domain.POSSale
			var completed *time.Time
			if err := rows.Scan(&srow.ID, &srow.BranchID, &srow.TicketNumber, &srow.Status, &srow.GrandTotal, &srow.TaxTotal,
				&srow.CustomerName, &srow.OperatorLabel, &srow.RequestInvoice, &completed, &srow.CreatedAt); err != nil {
				return err
			}
			srow.CompletedAt = completed
			out = append(out, srow)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Postgres) RequestSaleInvoice(ctx context.Context, orgRef, saleID string, req domain.RequestInvoiceRequest) (domain.FiscalInvoice, error) {
	req.RFC = strings.ToUpper(secure.PlainTextMax(req.RFC, 13))
	req.LegalName = secure.PlainTextMax(req.LegalName, 160)
	if req.RFC == "" || req.LegalName == "" {
		return domain.FiscalInvoice{}, errors.New("rfc and legal_name required")
	}
	sale, err := s.GetSale(ctx, orgRef, saleID)
	if err != nil {
		return domain.FiscalInvoice{}, err
	}
	if sale.Invoice != nil {
		return *sale.Invoice, nil
	}
	err = db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		fiscal, err := s.GetFiscalSettings(ctx, orgRef, sale.BranchID)
		if err != nil {
			return err
		}
		complete := domain.CompleteSaleRequest{
			InvoiceRFC:     req.RFC,
			InvoiceName:    req.LegalName,
			InvoiceRegime:  req.TaxRegime,
			InvoicePostal:  req.PostalCode,
			InvoiceUsoCFDI: req.UsoCFDI,
			InvoiceEmail:   req.Email,
		}
		saleUUID, err := uuid.Parse(saleID)
		if err != nil {
			return err
		}
		if err := insertInvoiceRequest(ctx, tx, orgID, saleUUID, fiscal, complete, sale.Subtotal, sale.TaxTotal, sale.GrandTotal); err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `UPDATE pos_sales SET request_invoice = TRUE WHERE id = $1::uuid`, saleID)
		return nil
	})
	if err != nil {
		return domain.FiscalInvoice{}, err
	}
	sale, err = s.GetSale(ctx, orgRef, saleID)
	if err != nil {
		return domain.FiscalInvoice{}, err
	}
	if sale.Invoice == nil {
		return domain.FiscalInvoice{}, errors.New("invoice_create_failed")
	}
	return *sale.Invoice, nil
}
