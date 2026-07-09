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
	"github.com/ramonisai2/NexusnodesERP/packages/go/secure"
)

func cardWaitStatusLabel(status string) string {
	switch status {
	case domain.CardWaitWaiting:
		return "En espera de terminal"
	case domain.CardWaitApproved:
		return "Aprobado"
	case domain.CardWaitDeclined:
		return "Rechazado"
	case domain.CardWaitCancelled:
		return "Cancelado"
	case domain.CardWaitExpired:
		return "Expirado"
	default:
		return status
	}
}

func (s *Postgres) CreateCardPaymentWait(ctx context.Context, req domain.CreateCardPaymentWaitRequest) (domain.CardPaymentWait, error) {
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.IdempotencyKey == "" {
		return domain.CardPaymentWait{}, errors.New("idempotency_key required")
	}
	if len(req.Lines) == 0 {
		return domain.CardPaymentWait{}, errors.New("lines required")
	}
	req.CustomerName = secure.PlainTextMax(req.CustomerName, 120)
	req.Notes = secure.PlainTextMax(req.Notes, 280)
	req.TerminalRef = secure.PlainTextMax(req.TerminalRef, 80)
	req.InvoiceRFC = strings.ToUpper(secure.PlainTextMax(req.InvoiceRFC, 13))
	req.InvoiceName = secure.PlainTextMax(req.InvoiceName, 160)
	req.InvoiceEmail = secure.PlainTextMax(req.InvoiceEmail, 120)
	if req.Currency == "" {
		req.Currency = "MXN"
	}
	if req.InvoiceUsoCFDI == "" {
		req.InvoiceUsoCFDI = "G01"
	}

	var waitID string
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
SELECT id::text FROM pos_card_payment_waits
WHERE org_id = $1::uuid AND idempotency_key = $2`, orgID, req.IdempotencyKey).Scan(&existing)
		if err == nil {
			waitID = existing
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
ORDER BY CASE warehouse_kind WHEN 'STORE' THEN 0 WHEN 'ARRIVAL' THEN 1 ELSE 2 END, code
LIMIT 1`, branchID).Scan(&whCode)
		}

		type built struct {
			SKU, Desc string
			Qty       float64
			Unit      float64
			LineTotal float64
		}
		var builtLines []built
		var amount float64
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
			err = tx.QueryRow(ctx, `
SELECT common_price, special_price, final_price, COALESCE(price_mode, 'COMMON')
FROM store_sku_labels
WHERE branch_id = $1::uuid AND sku_id = $2::uuid`, branchID, skuID).Scan(&commonP, &specialP, &finalP, &priceMode)
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("no price for sku %s", skuCode)
			}
			if err != nil {
				return err
			}
			lbl := domain.StoreLabel{CommonPrice: commonP, SpecialPrice: specialP, FinalPrice: finalP, PriceMode: priceMode}
			eff := lbl.ResolveEffectivePrice()
			if eff == nil {
				return fmt.Errorf("no price for sku %s", skuCode)
			}
			unit := round2(*eff)
			lineTotal := round2(unit * line.Quantity)
			builtLines = append(builtLines, built{SKU: skuCode, Desc: desc, Qty: line.Quantity, Unit: unit, LineTotal: lineTotal})
			amount = round2(amount + lineTotal)
		}
		if req.Amount > 0 {
			amount = round2(req.Amount)
		}
		if amount <= 0 {
			return errors.New("amount required")
		}

		payload := make([]domain.CardPaymentWaitLine, 0, len(builtLines))
		for _, b := range builtLines {
			payload = append(payload, domain.CardPaymentWaitLine{
				SKU: b.SKU, Description: b.Desc, Quantity: b.Qty, UnitPrice: b.Unit, LineTotal: b.LineTotal,
			})
		}
		linesJSON, _ := json.Marshal(payload)
		id := uuid.New()
		waitID = id.String()
		_, err = tx.Exec(ctx, `
INSERT INTO pos_card_payment_waits (
  id, org_id, branch_id, status, amount, currency, customer_name, lines, warehouse_code,
  request_invoice, invoice_rfc, invoice_name, invoice_email, invoice_uso_cfdi, notes,
  terminal_ref, created_by, operator_label, station_id, idempotency_key
) VALUES (
  $1::uuid, $2::uuid, $3::uuid, 'WAITING', $4, $5, $6, $7::jsonb, $8,
  $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
)`,
			id, orgID, branchID, amount, req.Currency, req.CustomerName, string(linesJSON), whCode,
			req.RequestInvoice, req.InvoiceRFC, req.InvoiceName, req.InvoiceEmail, req.InvoiceUsoCFDI, req.Notes,
			req.TerminalRef, req.CreatedBy, req.OperatorLabel, req.StationID, req.IdempotencyKey)
		return err
	})
	if err != nil {
		return domain.CardPaymentWait{}, err
	}
	return s.GetCardPaymentWait(ctx, req.OrgID, waitID)
}

func (s *Postgres) ListCardPaymentWaits(ctx context.Context, filter domain.CardPaymentWaitFilter) ([]domain.CardPaymentWait, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []domain.CardPaymentWait
	err := db.WithOrgTx(ctx, s.pool, filter.OrgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}
		q := `
SELECT w.id::text, w.org_id::text, b.code, w.status, w.amount::float8, w.currency, w.customer_name,
       w.lines, w.warehouse_code, w.request_invoice, w.invoice_rfc, w.invoice_name, w.invoice_email,
       w.invoice_uso_cfdi, w.notes, w.terminal_ref, w.auth_code, w.decline_reason, COALESCE(w.sale_id::text, ''),
       w.created_by, w.operator_label, w.station_id, w.confirmed_by, w.idempotency_key,
       w.created_at, w.updated_at, w.resolved_at
FROM pos_card_payment_waits w
JOIN branches b ON b.id = w.branch_id
WHERE w.org_id = $1::uuid`
		args := []any{orgID}
		argN := 2
		if filter.BranchCode != "" {
			q += fmt.Sprintf(` AND b.code = $%d`, argN)
			args = append(args, filter.BranchCode)
			argN++
		}
		if filter.Status != "" {
			q += fmt.Sprintf(` AND w.status = $%d`, argN)
			args = append(args, filter.Status)
			argN++
		}
		q += fmt.Sprintf(` ORDER BY w.created_at DESC LIMIT $%d`, argN)
		args = append(args, limit)

		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanCardWait(rows)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Postgres) GetCardPaymentWait(ctx context.Context, orgRef, waitID string) (domain.CardPaymentWait, error) {
	var out domain.CardPaymentWait
	err := db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
SELECT w.id::text, w.org_id::text, b.code, w.status, w.amount::float8, w.currency, w.customer_name,
       w.lines, w.warehouse_code, w.request_invoice, w.invoice_rfc, w.invoice_name, w.invoice_email,
       w.invoice_uso_cfdi, w.notes, w.terminal_ref, w.auth_code, w.decline_reason, COALESCE(w.sale_id::text, ''),
       w.created_by, w.operator_label, w.station_id, w.confirmed_by, w.idempotency_key,
       w.created_at, w.updated_at, w.resolved_at
FROM pos_card_payment_waits w
JOIN branches b ON b.id = w.branch_id
WHERE w.org_id = $1::uuid AND w.id = $2::uuid`, orgID, waitID)
		out, err = scanCardWait(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	if err != nil {
		return domain.CardPaymentWait{}, err
	}
	if out.SaleID != "" {
		if sale, err := s.GetSale(ctx, orgRef, out.SaleID); err == nil {
			out.Sale = &sale
		}
	}
	return out, nil
}

type cardWaitScanner interface {
	Scan(dest ...any) error
}

func scanCardWait(row cardWaitScanner) (domain.CardPaymentWait, error) {
	var out domain.CardPaymentWait
	var linesRaw []byte
	var resolved *time.Time
	err := row.Scan(
		&out.ID, &out.OrgID, &out.BranchID, &out.Status, &out.Amount, &out.Currency, &out.CustomerName,
		&linesRaw, &out.WarehouseCode, &out.RequestInvoice, &out.InvoiceRFC, &out.InvoiceName, &out.InvoiceEmail,
		&out.InvoiceUsoCFDI, &out.Notes, &out.TerminalRef, &out.AuthCode, &out.DeclineReason, &out.SaleID,
		&out.CreatedBy, &out.OperatorLabel, &out.StationID, &out.ConfirmedBy, &out.IdempotencyKey,
		&out.CreatedAt, &out.UpdatedAt, &resolved,
	)
	if err != nil {
		return out, err
	}
	_ = json.Unmarshal(linesRaw, &out.Lines)
	if out.Lines == nil {
		out.Lines = []domain.CardPaymentWaitLine{}
	}
	out.StatusLabel = cardWaitStatusLabel(out.Status)
	out.ResolvedAt = resolved
	return out, nil
}

func (s *Postgres) ConfirmCardPaymentWait(ctx context.Context, orgRef, waitID string, req domain.ConfirmCardPaymentWaitRequest) (domain.CardPaymentWait, error) {
	wait, err := s.GetCardPaymentWait(ctx, orgRef, waitID)
	if err != nil {
		return domain.CardPaymentWait{}, err
	}
	if wait.Status != domain.CardWaitWaiting {
		return domain.CardPaymentWait{}, errors.New("wait_not_pending")
	}
	req.AuthCode = secure.PlainTextMax(req.AuthCode, 40)
	req.TerminalRef = secure.PlainTextMax(req.TerminalRef, 80)
	req.DeclineReason = secure.PlainTextMax(req.DeclineReason, 200)

	if !req.Approved {
		err := db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
			if err := db.SetRLSBypass(ctx, tx, true); err != nil {
				return err
			}
			orgID, err := resolveOrgID(ctx, tx, orgRef)
			if err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `
UPDATE pos_card_payment_waits
SET status = 'DECLINED',
    auth_code = $3,
    terminal_ref = CASE WHEN $4 = '' THEN terminal_ref ELSE $4 END,
    decline_reason = $5,
    confirmed_by = $6,
    operator_label = CASE WHEN $7 = '' THEN operator_label ELSE $7 END,
    resolved_at = now(),
    updated_at = now()
WHERE org_id = $1::uuid AND id = $2::uuid AND status = 'WAITING'`,
				orgID, waitID, req.AuthCode, req.TerminalRef, req.DeclineReason, req.ConfirmedBy, req.OperatorLabel)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return errors.New("wait_not_pending")
			}
			return nil
		})
		if err != nil {
			return domain.CardPaymentWait{}, err
		}
		return s.GetCardPaymentWait(ctx, orgRef, waitID)
	}

	lines := make([]domain.POSLineInput, 0, len(wait.Lines))
	for _, l := range wait.Lines {
		lines = append(lines, domain.POSLineInput{SKU: l.SKU, Quantity: l.Quantity})
	}
	ref := req.TerminalRef
	if ref == "" {
		ref = wait.TerminalRef
	}
	if req.AuthCode != "" {
		if ref != "" {
			ref = ref + "/" + req.AuthCode
		} else {
			ref = req.AuthCode
		}
	}
	sale, err := s.CompleteSale(ctx, domain.CompleteSaleRequest{
		OrgID:          orgRef,
		BranchID:       wait.BranchID,
		WarehouseID:    wait.WarehouseCode,
		CustomerName:   wait.CustomerName,
		Lines:          lines,
		Payments:       []domain.POSPaymentInput{{Method: domain.PaymentCard, Amount: wait.Amount, Reference: ref}},
		RequestInvoice: wait.RequestInvoice,
		InvoiceRFC:     wait.InvoiceRFC,
		InvoiceName:    wait.InvoiceName,
		InvoiceEmail:   wait.InvoiceEmail,
		InvoiceUsoCFDI: wait.InvoiceUsoCFDI,
		Notes:          wait.Notes,
		IdempotencyKey: "cardwait-" + wait.ID,
		CashierSub:     req.ConfirmedBy,
		OperatorLabel:  firstNonEmpty(req.OperatorLabel, wait.OperatorLabel),
		StationID:      wait.StationID,
	})
	if err != nil {
		return domain.CardPaymentWait{}, err
	}

	err = db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
UPDATE pos_card_payment_waits
SET status = 'APPROVED',
    sale_id = $3::uuid,
    auth_code = $4,
    terminal_ref = CASE WHEN $5 = '' THEN terminal_ref ELSE $5 END,
    confirmed_by = $6,
    operator_label = CASE WHEN $7 = '' THEN operator_label ELSE $7 END,
    resolved_at = now(),
    updated_at = now()
WHERE org_id = $1::uuid AND id = $2::uuid`,
			orgID, waitID, sale.ID, req.AuthCode, req.TerminalRef, req.ConfirmedBy, req.OperatorLabel)
		return err
	})
	if err != nil {
		return domain.CardPaymentWait{}, err
	}
	out, err := s.GetCardPaymentWait(ctx, orgRef, waitID)
	if err != nil {
		return domain.CardPaymentWait{}, err
	}
	out.Sale = &sale
	return out, nil
}

func (s *Postgres) CancelCardPaymentWait(ctx context.Context, orgRef, waitID string, req domain.CancelCardPaymentWaitRequest) (domain.CardPaymentWait, error) {
	req.Reason = secure.PlainTextMax(req.Reason, 200)
	err := db.WithOrgTx(ctx, s.pool, orgRef, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
UPDATE pos_card_payment_waits
SET status = 'CANCELLED',
    decline_reason = $3,
    confirmed_by = $4,
    operator_label = CASE WHEN $5 = '' THEN operator_label ELSE $5 END,
    resolved_at = now(),
    updated_at = now()
WHERE org_id = $1::uuid AND id = $2::uuid AND status = 'WAITING'`,
			orgID, waitID, req.Reason, req.CancelledBy, req.OperatorLabel)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("wait_not_pending")
		}
		return nil
	})
	if err != nil {
		return domain.CardPaymentWait{}, err
	}
	return s.GetCardPaymentWait(ctx, orgRef, waitID)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
