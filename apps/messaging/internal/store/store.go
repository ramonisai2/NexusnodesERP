package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

var (
	ErrNotFound = errors.New("not_found")
	ErrForbidden = errors.New("forbidden")
)

const (
	PriorityLow    = "LOW"
	PriorityNormal = "NORMAL"
	PriorityHigh   = "HIGH"
	PriorityUrgent = "URGENT"

	KindMessage      = "MESSAGE"
	KindAnnouncement = "ANNOUNCEMENT"

	FolderInbox   = "INBOX"
	FolderSent    = "SENT"
	FolderArchive = "ARCHIVE"
)

func PriorityColor(p string) string {
	switch strings.ToUpper(p) {
	case PriorityLow:
		return "gray"
	case PriorityHigh:
		return "amber"
	case PriorityUrgent:
		return "red"
	default:
		return "blue"
	}
}

func ValidPriority(p string) bool {
	switch strings.ToUpper(strings.TrimSpace(p)) {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent:
		return true
	default:
		return false
	}
}

type Message struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	FromSub      string     `json:"from_sub"`
	FromOperator string     `json:"from_operator,omitempty"`
	ToSubs       []string   `json:"to_subs,omitempty"`
	Subject      string     `json:"subject"`
	Body         string     `json:"body"`
	Priority     string     `json:"priority"`
	PriorityColor string    `json:"priority_color"`
	Kind         string     `json:"kind"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Expired      bool       `json:"expired,omitempty"`
	ColorToken   string     `json:"color_token,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ReadAt       *time.Time `json:"read_at,omitempty"`
	Folder       string     `json:"folder,omitempty"`
	RecipientID  string     `json:"recipient_id,omitempty"`
}

type SendInput struct {
	OrgRef       string
	FromSub      string
	FromOperator string
	ToSubs       []string
	Subject      string
	Body         string
	Priority     string
	Kind         string
	ExpiresAt    *time.Time
	ColorToken   string
}

type ListFilter struct {
	OrgRef string
	Sub    string
	Folder string
	Kind   string
	Limit  int
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Send(ctx context.Context, in SendInput) (Message, error) {
	in.Subject = strings.TrimSpace(in.Subject)
	in.Body = strings.TrimSpace(in.Body)
	if in.Subject == "" {
		return Message{}, errors.New("subject_required")
	}
	priority := strings.ToUpper(strings.TrimSpace(in.Priority))
	if priority == "" {
		priority = PriorityNormal
	}
	if !ValidPriority(priority) {
		return Message{}, errors.New("invalid_priority")
	}
	kind := strings.ToUpper(strings.TrimSpace(in.Kind))
	if kind == "" {
		kind = KindMessage
	}
	if kind != KindMessage && kind != KindAnnouncement {
		return Message{}, errors.New("invalid_kind")
	}
	if kind == KindAnnouncement && in.ExpiresAt == nil {
		exp := time.Now().UTC().Add(7 * 24 * time.Hour)
		in.ExpiresAt = &exp
	}
	color := strings.TrimSpace(in.ColorToken)
	if color == "" {
		color = PriorityColor(priority)
	}

	var out Message
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, in.OrgRef)
		if err != nil {
			return err
		}
		if err := db.SetOrgContext(ctx, tx, orgID); err != nil {
			return err
		}

		recipients := uniqueNonEmpty(in.ToSubs)
		if kind == KindAnnouncement && len(recipients) == 0 {
			rows, err := tx.Query(ctx, `SELECT idp_sub FROM users WHERE org_id = $1::uuid`, orgID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var sub string
				if err := rows.Scan(&sub); err != nil {
					rows.Close()
					return err
				}
				recipients = append(recipients, sub)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}
		if len(recipients) == 0 {
			return errors.New("recipients_required")
		}

		id := uuid.New()
		now := time.Now().UTC()
		_, err = tx.Exec(ctx, `
INSERT INTO mail_messages (
  id, org_id, from_sub, from_operator, subject, body, priority, kind, expires_at, color_token, created_at
) VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			id, orgID, in.FromSub, strings.TrimSpace(in.FromOperator), in.Subject, in.Body,
			priority, kind, in.ExpiresAt, color, now)
		if err != nil {
			return err
		}

		// Sender copy in SENT
		_, err = tx.Exec(ctx, `
INSERT INTO mail_recipients (id, message_id, org_id, to_sub, folder, read_at, created_at)
VALUES ($1, $2, $3::uuid, $4, 'SENT', $5, $5)
ON CONFLICT (message_id, to_sub, folder) DO NOTHING`,
			uuid.New(), id, orgID, in.FromSub, now)
		if err != nil {
			return err
		}

		for _, to := range recipients {
			_, err = tx.Exec(ctx, `
INSERT INTO mail_recipients (id, message_id, org_id, to_sub, folder, created_at)
VALUES ($1, $2, $3::uuid, $4, 'INBOX', $5)
ON CONFLICT (message_id, to_sub, folder) DO NOTHING`,
				uuid.New(), id, orgID, to, now)
			if err != nil {
				return err
			}
		}

		out = Message{
			ID: id.String(), OrgID: orgID, FromSub: in.FromSub, FromOperator: in.FromOperator,
			ToSubs: recipients, Subject: in.Subject, Body: in.Body, Priority: priority,
			PriorityColor: color, Kind: kind, ExpiresAt: in.ExpiresAt, ColorToken: color, CreatedAt: now,
			Folder: FolderSent,
		}
		return nil
	})
	return out, err
}

func (s *Store) List(ctx context.Context, filter ListFilter) ([]Message, error) {
	folder := strings.ToUpper(strings.TrimSpace(filter.Folder))
	if folder == "" {
		folder = FolderInbox
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	var out []Message
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, filter.OrgRef)
		if err != nil {
			return err
		}
		q := `
SELECT m.id::text, m.org_id::text, m.from_sub, m.from_operator, m.subject, m.body,
       m.priority, COALESCE(NULLIF(m.color_token,''), ''), m.kind, m.expires_at, m.created_at,
       r.id::text, r.folder, r.read_at,
       CASE WHEN m.expires_at IS NOT NULL AND m.expires_at < now() THEN true ELSE false END
FROM mail_recipients r
JOIN mail_messages m ON m.id = r.message_id
WHERE r.org_id = $1::uuid AND r.to_sub = $2 AND r.folder = $3`
		args := []any{orgID, filter.Sub, folder}
		n := 4
		if filter.Kind != "" {
			q += fmt.Sprintf(` AND m.kind = $%d`, n)
			args = append(args, strings.ToUpper(filter.Kind))
			n++
		}
		if folder == FolderInbox {
			// Hide expired announcements from inbox listing.
			q += ` AND (m.expires_at IS NULL OR m.expires_at > now() OR m.kind <> 'ANNOUNCEMENT')`
		}
		q += fmt.Sprintf(` ORDER BY m.created_at DESC LIMIT $%d`, n)
		args = append(args, limit)

		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m Message
			var color string
			var expired bool
			if err := rows.Scan(
				&m.ID, &m.OrgID, &m.FromSub, &m.FromOperator, &m.Subject, &m.Body,
				&m.Priority, &color, &m.Kind, &m.ExpiresAt, &m.CreatedAt,
				&m.RecipientID, &m.Folder, &m.ReadAt, &expired,
			); err != nil {
				return err
			}
			if color == "" {
				color = PriorityColor(m.Priority)
			}
			m.ColorToken = color
			m.PriorityColor = color
			m.Expired = expired
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) Get(ctx context.Context, orgRef, sub, messageID string) (Message, error) {
	var out Message
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
SELECT m.id::text, m.org_id::text, m.from_sub, m.from_operator, m.subject, m.body,
       m.priority, COALESCE(NULLIF(m.color_token,''), ''), m.kind, m.expires_at, m.created_at,
       r.id::text, r.folder, r.read_at,
       CASE WHEN m.expires_at IS NOT NULL AND m.expires_at < now() THEN true ELSE false END
FROM mail_recipients r
JOIN mail_messages m ON m.id = r.message_id
WHERE r.org_id = $1::uuid AND r.to_sub = $2 AND m.id = $3::uuid
ORDER BY CASE r.folder WHEN 'INBOX' THEN 0 WHEN 'SENT' THEN 1 ELSE 2 END
LIMIT 1`, orgID, sub, messageID)
		var color string
		var expired bool
		err = row.Scan(
			&out.ID, &out.OrgID, &out.FromSub, &out.FromOperator, &out.Subject, &out.Body,
			&out.Priority, &color, &out.Kind, &out.ExpiresAt, &out.CreatedAt,
			&out.RecipientID, &out.Folder, &out.ReadAt, &expired,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if color == "" {
			color = PriorityColor(out.Priority)
		}
		out.ColorToken = color
		out.PriorityColor = color
		out.Expired = expired
		return nil
	})
	return out, err
}

func (s *Store) MarkRead(ctx context.Context, orgRef, sub, messageID string) (Message, error) {
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		ct, err := tx.Exec(ctx, `
UPDATE mail_recipients
SET read_at = COALESCE(read_at, now())
WHERE org_id = $1::uuid AND to_sub = $2 AND message_id = $3::uuid AND folder = 'INBOX'`,
			orgID, sub, messageID)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return Message{}, err
	}
	return s.Get(ctx, orgRef, sub, messageID)
}

func (s *Store) Archive(ctx context.Context, orgRef, sub, messageID string) (Message, error) {
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		var inboxID string
		err = tx.QueryRow(ctx, `
SELECT id::text FROM mail_recipients
WHERE org_id = $1::uuid AND to_sub = $2 AND message_id = $3::uuid AND folder = 'INBOX'`,
			orgID, sub, messageID).Scan(&inboxID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		_, err = tx.Exec(ctx, `
INSERT INTO mail_recipients (id, message_id, org_id, to_sub, folder, read_at, archived_at, created_at)
SELECT $1, message_id, org_id, to_sub, 'ARCHIVE', COALESCE(read_at, $2), $2, created_at
FROM mail_recipients WHERE id = $3::uuid
ON CONFLICT (message_id, to_sub, folder) DO UPDATE
SET archived_at = EXCLUDED.archived_at, read_at = COALESCE(mail_recipients.read_at, EXCLUDED.read_at)`,
			uuid.New(), now, inboxID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM mail_recipients WHERE id = $1::uuid`, inboxID)
		return err
	})
	if err != nil {
		return Message{}, err
	}
	return s.Get(ctx, orgRef, sub, messageID)
}

func (s *Store) UnreadCount(ctx context.Context, orgRef, sub string) (int, error) {
	var n int
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
SELECT count(*)::int
FROM mail_recipients r
JOIN mail_messages m ON m.id = r.message_id
WHERE r.org_id = $1::uuid AND r.to_sub = $2 AND r.folder = 'INBOX' AND r.read_at IS NULL
  AND (m.expires_at IS NULL OR m.expires_at > now() OR m.kind <> 'ANNOUNCEMENT')`,
			orgID, sub).Scan(&n)
	})
	return n, err
}

func (s *Store) Directory(ctx context.Context, orgRef string) ([]map[string]string, error) {
	var out []map[string]string
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
SELECT idp_sub, COALESCE(display_name, idp_sub), COALESCE(email, '')
FROM users WHERE org_id = $1::uuid ORDER BY display_name, idp_sub LIMIT 100`, orgID)
		if err != nil {
			// display_name may not exist in older schemas — fallback
			rows, err = tx.Query(ctx, `
SELECT idp_sub, idp_sub, COALESCE(email, '')
FROM users WHERE org_id = $1::uuid ORDER BY idp_sub LIMIT 100`, orgID)
			if err != nil {
				return err
			}
		}
		defer rows.Close()
		for rows.Next() {
			var sub, name, email string
			if err := rows.Scan(&sub, &name, &email); err != nil {
				return err
			}
			out = append(out, map[string]string{"sub": sub, "name": name, "email": email})
		}
		return rows.Err()
	})
	return out, err
}

func resolveOrg(ctx context.Context, tx pgx.Tx, orgRef string) (string, error) {
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
		return "", fmt.Errorf("org not found")
	}
	return id, err
}

func uniqueNonEmpty(in []string) []string {
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
