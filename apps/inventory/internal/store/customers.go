package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"golang.org/x/crypto/bcrypt"
	"github.com/ramonisai2/NexusnodesERP/packages/go/secure"
)

func (s *Postgres) ResolveOrgByStorefrontSlug(ctx context.Context, slug string) (string, string, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return "", "", domain.ErrNotFound
	}
	var orgID, branchCode string
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
SELECT s.org_id::text, b.code
FROM branch_storefront_settings s
JOIN branches b ON b.id = s.branch_id
WHERE s.public_slug = $1 AND s.published = TRUE`, slug).Scan(&orgID, &branchCode)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	return orgID, branchCode, err
}

func (s *Postgres) RegisterCustomer(ctx context.Context, req domain.RegisterCustomerRequest) (domain.Customer, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		return domain.Customer{}, errors.New("email required")
	}
	if len(req.Password) < 8 {
		return domain.Customer{}, errors.New("password must be at least 8 characters")
	}
	if msg := secure.RejectIfInjection("display_name", req.DisplayName); msg != "" {
		return domain.Customer{}, errors.New(msg)
	}
	if msg := secure.RejectIfInjection("phone", req.Phone); msg != "" {
		return domain.Customer{}, errors.New(msg)
	}
	name := secure.PlainTextMax(req.DisplayName, 120)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	req.Phone = secure.PlainTextMax(req.Phone, 40)

	orgRef := strings.TrimSpace(req.OrgID)
	branchHint := strings.TrimSpace(req.PreferredBranch)
	if orgRef == "" {
		if strings.TrimSpace(req.StorefrontSlug) == "" {
			return domain.Customer{}, errors.New("storefront_slug or org_id required")
		}
		oid, branch, err := s.ResolveOrgByStorefrontSlug(ctx, req.StorefrontSlug)
		if err != nil {
			return domain.Customer{}, err
		}
		orgRef = oid
		if branchHint == "" {
			branchHint = branch
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.Customer{}, err
	}

	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.Customer{}, err
	}
	defer tx.Rollback(ctx)

	var preferred any
	if branchHint != "" {
		bid, err := resolveBranchID(ctx, tx, orgID, branchHint)
		if err == nil {
			preferred = bid
		}
	}

	id := uuid.New()
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
INSERT INTO customers (
  id, org_id, email, phone, display_name, password_hash, status, preferred_branch_id, created_at, updated_at
) VALUES ($1, $2::uuid, $3, $4, $5, $6, 'ACTIVE', $7, $8, $8)`,
		id, orgID, email, strings.TrimSpace(req.Phone), name, string(hash), preferred, now)
	if err != nil {
		if strings.Contains(err.Error(), "customers_org_id_email_key") || strings.Contains(err.Error(), "unique") {
			return domain.Customer{}, errors.New("email already registered")
		}
		return domain.Customer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Customer{}, err
	}
	return s.GetCustomer(ctx, orgID, id.String())
}

func (s *Postgres) AuthenticateCustomer(ctx context.Context, req domain.LoginCustomerRequest) (domain.Customer, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		return domain.Customer{}, errors.New("email and password required")
	}
	orgRef := strings.TrimSpace(req.OrgID)
	if orgRef == "" {
		if strings.TrimSpace(req.StorefrontSlug) == "" {
			return domain.Customer{}, errors.New("storefront_slug or org_id required")
		}
		oid, _, err := s.ResolveOrgByStorefrontSlug(ctx, req.StorefrontSlug)
		if err != nil {
			return domain.Customer{}, err
		}
		orgRef = oid
	}

	var out domain.Customer
	var hash string
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
SELECT c.id::text, c.org_id::text, c.email, c.phone, c.display_name, c.status,
       COALESCE(b.code, ''), c.password_hash, c.created_at
FROM customers c
LEFT JOIN branches b ON b.id = c.preferred_branch_id
WHERE c.org_id = $1::uuid AND lower(c.email) = $2`, orgID, email).Scan(
			&out.ID, &out.OrgID, &out.Email, &out.Phone, &out.DisplayName, &out.Status,
			&out.PreferredBranchID, &hash, &out.CreatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	if err != nil {
		return domain.Customer{}, err
	}
	if out.Status != domain.CustomerStatusActive {
		return domain.Customer{}, errors.New("account not active")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		return domain.Customer{}, errors.New("invalid credentials")
	}
	cards, _ := s.ListCustomerCards(ctx, out.OrgID, out.ID)
	out.Cards = cards
	return out, nil
}

func (s *Postgres) GetCustomer(ctx context.Context, orgRef, customerID string) (domain.Customer, error) {
	var out domain.Customer
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
SELECT c.id::text, c.org_id::text, c.email, c.phone, c.display_name, c.status,
       COALESCE(b.code, ''), c.created_at
FROM customers c
LEFT JOIN branches b ON b.id = c.preferred_branch_id
WHERE c.org_id = $1::uuid AND c.id = $2::uuid`, orgID, customerID).Scan(
			&out.ID, &out.OrgID, &out.Email, &out.Phone, &out.DisplayName, &out.Status,
			&out.PreferredBranchID, &out.CreatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	if err != nil {
		return domain.Customer{}, err
	}
	cards, err := s.ListCustomerCards(ctx, out.OrgID, out.ID)
	if err != nil {
		return domain.Customer{}, err
	}
	out.Cards = cards
	return out, nil
}

func (s *Postgres) ListCustomers(ctx context.Context, filter domain.CustomerFilter) ([]domain.Customer, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	var out []domain.Customer
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}
		q := `
SELECT c.id::text, c.org_id::text, c.email, c.phone, c.display_name, c.status,
       COALESCE(b.code, ''), c.created_at
FROM customers c
LEFT JOIN branches b ON b.id = c.preferred_branch_id
WHERE c.org_id = $1::uuid`
		args := []any{orgID}
		n := 2
		if filter.Status != "" {
			q += fmt.Sprintf(` AND c.status = $%d`, n)
			args = append(args, strings.ToUpper(filter.Status))
			n++
		}
		if qstr := strings.TrimSpace(filter.Query); qstr != "" {
			q += fmt.Sprintf(` AND (c.email ILIKE $%d OR c.display_name ILIKE $%d OR c.phone ILIKE $%d)`, n, n, n)
			args = append(args, "%"+qstr+"%")
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
			var c domain.Customer
			if err := rows.Scan(
				&c.ID, &c.OrgID, &c.Email, &c.Phone, &c.DisplayName, &c.Status,
				&c.PreferredBranchID, &c.CreatedAt,
			); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Postgres) ListCustomerCards(ctx context.Context, orgRef, customerID string) ([]domain.CustomerCard, error) {
	var out []domain.CustomerCard
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
SELECT id::text, org_id::text, customer_id::text, card_kind, card_code, label, status,
       issued_by, blocked_reason, blocked_at, last_seen_at, created_at
FROM customer_cards
WHERE org_id = $1::uuid AND customer_id = $2::uuid
ORDER BY created_at DESC`, orgID, customerID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			card, err := scanCustomerCard(rows)
			if err != nil {
				return err
			}
			out = append(out, card)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Postgres) CreateCustomerCard(ctx context.Context, req domain.CreateCustomerCardRequest) (domain.CustomerCard, error) {
	kind := strings.ToUpper(strings.TrimSpace(req.CardKind))
	if !domain.ValidCardKind(kind) {
		return domain.CustomerCard{}, errors.New("card_kind must be BARCODE or CHIP")
	}
	code := strings.TrimSpace(req.CardCode)
	if code == "" {
		code = generateCardCode(kind)
	}
	code = strings.ToUpper(code)
	if len(code) < 4 {
		return domain.CustomerCard{}, errors.New("card_code too short")
	}
	if msg := secure.RejectIfInjection("card_code", code); msg != "" {
		return domain.CustomerCard{}, errors.New(msg)
	}
	if msg := secure.RejectIfInjection("label", req.Label); msg != "" {
		return domain.CustomerCard{}, errors.New(msg)
	}
	if strings.TrimSpace(req.CustomerID) == "" {
		return domain.CustomerCard{}, errors.New("customer_id required")
	}

	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, req.OrgID)
	})
	if err != nil {
		return domain.CustomerCard{}, err
	}
	defer tx.Rollback(ctx)

	var ok string
	err = tx.QueryRow(ctx, `
SELECT id::text FROM customers WHERE org_id = $1::uuid AND id = $2::uuid AND status = 'ACTIVE'`,
		orgID, req.CustomerID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomerCard{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.CustomerCard{}, err
	}

	id := uuid.New()
	now := time.Now().UTC()
	label := secure.PlainTextMax(req.Label, 80)
	if label == "" {
		if kind == domain.CardKindChip {
			label = "Tarjeta chip"
		} else {
			label = "Tarjeta código de barras"
		}
	}
	_, err = tx.Exec(ctx, `
INSERT INTO customer_cards (
  id, org_id, customer_id, card_kind, card_code, label, status, issued_by, created_at, updated_at
) VALUES ($1, $2::uuid, $3::uuid, $4, $5, $6, 'ACTIVE', $7, $8, $8)`,
		id, orgID, req.CustomerID, kind, code, label, strings.TrimSpace(req.IssuedBy), now)
	if err != nil {
		if strings.Contains(err.Error(), "customer_cards_org_id_card_code_key") || strings.Contains(err.Error(), "unique") {
			return domain.CustomerCard{}, errors.New("card_code already exists")
		}
		return domain.CustomerCard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CustomerCard{}, err
	}
	return s.getCustomerCard(ctx, orgID, id.String())
}

func (s *Postgres) BlockCustomerCard(ctx context.Context, orgRef, cardID string, req domain.BlockCardRequest) (domain.CustomerCard, error) {
	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, orgRef)
	})
	if err != nil {
		return domain.CustomerCard{}, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
SELECT status FROM customer_cards WHERE org_id = $1::uuid AND id = $2::uuid FOR UPDATE`,
		orgID, cardID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomerCard{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.CustomerCard{}, err
	}
	if status == domain.CardStatusBlocked || status == domain.CardStatusLost {
		_ = tx.Commit(ctx)
		return s.getCustomerCard(ctx, orgID, cardID)
	}
	now := time.Now().UTC()
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "Bloqueada por el titular o la tienda"
	}
	_, err = tx.Exec(ctx, `
UPDATE customer_cards
SET status = 'BLOCKED', blocked_reason = $1, blocked_at = $2, updated_at = $2
WHERE id = $3::uuid`, reason, now, cardID)
	if err != nil {
		return domain.CustomerCard{}, err
	}
	_ = req.Actor
	if err := tx.Commit(ctx); err != nil {
		return domain.CustomerCard{}, err
	}
	return s.getCustomerCard(ctx, orgID, cardID)
}

func (s *Postgres) LookupCustomerCard(ctx context.Context, orgRef, cardCode string) (domain.CardLookupResult, error) {
	code := strings.ToUpper(strings.TrimSpace(cardCode))
	if code == "" {
		return domain.CardLookupResult{}, errors.New("card_code required")
	}
	var card domain.CustomerCard
	var customerID string
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
SELECT id::text, org_id::text, customer_id::text, card_kind, card_code, label, status,
       issued_by, blocked_reason, blocked_at, last_seen_at, created_at
FROM customer_cards
WHERE org_id = $1::uuid AND upper(card_code) = $2`, orgID, code)
		card, err = scanCustomerCard(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		customerID = card.CustomerID
		_, _ = tx.Exec(ctx, `
UPDATE customer_cards SET last_seen_at = now(), updated_at = now() WHERE id = $1::uuid`, card.ID)
		return nil
	})
	if err != nil {
		return domain.CardLookupResult{}, err
	}
	cust, err := s.GetCustomer(ctx, card.OrgID, customerID)
	if err != nil {
		return domain.CardLookupResult{}, err
	}
	// Refresh card after last_seen update.
	fresh, _ := s.getCustomerCard(ctx, card.OrgID, card.ID)
	if fresh.ID != "" {
		card = fresh
	}
	return domain.CardLookupResult{Card: card, Customer: cust}, nil
}

func (s *Postgres) getCustomerCard(ctx context.Context, orgRef, cardID string) (domain.CustomerCard, error) {
	var out domain.CustomerCard
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
SELECT id::text, org_id::text, customer_id::text, card_kind, card_code, label, status,
       issued_by, blocked_reason, blocked_at, last_seen_at, created_at
FROM customer_cards
WHERE org_id = $1::uuid AND id = $2::uuid`, orgID, cardID)
		out, err = scanCustomerCard(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	return out, err
}

type cardScanner interface {
	Scan(dest ...any) error
}

func scanCustomerCard(row cardScanner) (domain.CustomerCard, error) {
	var c domain.CustomerCard
	var blocked, lastSeen *time.Time
	err := row.Scan(
		&c.ID, &c.OrgID, &c.CustomerID, &c.CardKind, &c.CardCode, &c.Label, &c.Status,
		&c.IssuedBy, &c.BlockedReason, &blocked, &lastSeen, &c.CreatedAt,
	)
	if err != nil {
		return domain.CustomerCard{}, err
	}
	c.BlockedAt = blocked
	c.LastSeenAt = lastSeen
	c.CardKindLabel = domain.CardKindLabelES(c.CardKind)
	return c, nil
}

func generateCardCode(kind string) string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	hexCode := strings.ToUpper(hex.EncodeToString(buf))
	if kind == domain.CardKindChip {
		return "CHIP-" + hexCode
	}
	// Numeric-ish barcode: prefix + hex as digits substitute
	return "NX" + hexCode
}
