package sessions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

var ErrNotFound = errors.New("session_not_found")

type Station struct {
	ID            string    `json:"id"`
	UserSub       string    `json:"user_sub"`
	OperatorLabel string    `json:"operator_label"`
	StationID     string    `json:"station_id"`
	JTI           string    `json:"jti,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	CreatedAt     time.Time `json:"created_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// OpenStation registers a concurrent operator seat under a shared account.
func (s *Store) OpenStation(ctx context.Context, orgRef, idpSub, operatorLabel, stationID, jti string, ttl time.Duration) (Station, error) {
	operatorLabel = strings.TrimSpace(operatorLabel)
	if operatorLabel == "" {
		return Station{}, errors.New("operator_label_required")
	}
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	var out Station
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		var userID, orgID string
		err := tx.QueryRow(ctx, `
SELECT id::text, org_id::text FROM users WHERE idp_sub = $1
ORDER BY created_at LIMIT 1`, idpSub).Scan(&userID, &orgID)
		if errors.Is(err, pgx.ErrNoRows) {
			// Dev personas may not exist in users — create a lightweight session-less station id.
			out = Station{
				ID:            uuid.NewString(),
				UserSub:       idpSub,
				OperatorLabel: operatorLabel,
				StationID:     strings.TrimSpace(stationID),
				JTI:           jti,
				ExpiresAt:     time.Now().UTC().Add(ttl),
				LastSeenAt:    time.Now().UTC(),
				CreatedAt:     time.Now().UTC(),
			}
			return nil
		}
		if err != nil {
			return err
		}
		id := uuid.New()
		now := time.Now().UTC()
		exp := now.Add(ttl)
		_, err = tx.Exec(ctx, `
INSERT INTO sessions (
  id, user_id, org_id, device_hash, expires_at, operator_label, station_id, jti, last_seen_at, created_at
) VALUES ($1, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $9)`,
			id, userID, orgID, strings.TrimSpace(stationID), exp, operatorLabel, strings.TrimSpace(stationID), jti, now)
		if err != nil {
			return err
		}
		out = Station{
			ID: id.String(), UserSub: idpSub, OperatorLabel: operatorLabel,
			StationID: strings.TrimSpace(stationID), JTI: jti,
			ExpiresAt: exp, LastSeenAt: now, CreatedAt: now,
		}
		return nil
	})
	return out, err
}

func (s *Store) ListActive(ctx context.Context, idpSub string) ([]Station, error) {
	var out []Station
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
SELECT s.id::text, u.idp_sub, COALESCE(s.operator_label,''), COALESCE(s.station_id,''),
       COALESCE(s.jti,''), s.expires_at, s.last_seen_at, s.created_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE u.idp_sub = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
ORDER BY s.created_at DESC
LIMIT 50`, idpSub)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var st Station
			if err := rows.Scan(&st.ID, &st.UserSub, &st.OperatorLabel, &st.StationID, &st.JTI, &st.ExpiresAt, &st.LastSeenAt, &st.CreatedAt); err != nil {
				return err
			}
			out = append(out, st)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) Touch(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if _, err := uuid.Parse(sessionID); err != nil {
		return nil
	}
	return db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
UPDATE sessions SET last_seen_at = now()
WHERE id = $1::uuid AND revoked_at IS NULL`, sessionID)
		return err
	})
}

func (s *Store) Revoke(ctx context.Context, sessionID, idpSub string) error {
	return db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		ct, err := tx.Exec(ctx, `
UPDATE sessions s SET revoked_at = now()
FROM users u
WHERE s.id = $1::uuid AND s.user_id = u.id AND u.idp_sub = $2 AND s.revoked_at IS NULL`,
			sessionID, idpSub)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}
