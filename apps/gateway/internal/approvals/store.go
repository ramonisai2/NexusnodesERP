package approvals

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
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

var (
	ErrNotFound      = errors.New("approval_not_found")
	ErrNotPending    = errors.New("approval_not_pending")
	ErrAlreadyExists = errors.New("approval_already_pending")
)

type Request struct {
	ID                   string          `json:"id"`
	OrgID                string          `json:"org_id"`
	ActionCode           string          `json:"action_code"`
	ResourceType         string          `json:"resource_type"`
	ResourceID           string          `json:"resource_id"`
	BranchID             string          `json:"branch_id"`
	WarehouseID          string          `json:"warehouse_id"`
	Payload              json.RawMessage `json:"payload"`
	Summary              string          `json:"summary"`
	Status               string          `json:"status"`
	RequestedBySub       string          `json:"requested_by_sub"`
	RequestedByOperator  string          `json:"requested_by_operator"`
	RequestedBySession   string          `json:"requested_by_session,omitempty"`
	RequestedAt          time.Time       `json:"requested_at"`
	DecidedBySub         string          `json:"decided_by_sub,omitempty"`
	DecidedByOperator    string          `json:"decided_by_operator,omitempty"`
	DecidedAt            *time.Time      `json:"decided_at,omitempty"`
	DecisionReason       string          `json:"decision_reason,omitempty"`
	ResultRef            string          `json:"result_ref,omitempty"`
}

type CreateInput struct {
	OrgRef              string
	ActionCode          string
	ResourceType        string
	ResourceID          string
	BranchID            string
	WarehouseID         string
	Payload             any
	Summary             string
	RequestedBySub      string
	RequestedByOperator string
	RequestedBySession  string
}

type DecideInput struct {
	OrgRef            string
	RequestID         string
	Approve           bool
	DecidedBySub      string
	DecidedByOperator string
	Reason            string
	ResultRef         string
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Create(ctx context.Context, in CreateInput) (Request, error) {
	raw, err := json.Marshal(in.Payload)
	if err != nil {
		return Request{}, err
	}
	var out Request
	err = db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, in.OrgRef)
		if err != nil {
			return err
		}
		var existing string
		err = tx.QueryRow(ctx, `
SELECT id::text FROM approval_requests
WHERE org_id = $1::uuid AND resource_type = $2 AND resource_id = $3 AND action_code = $4 AND status = 'PENDING'`,
			orgID, in.ResourceType, in.ResourceID, in.ActionCode).Scan(&existing)
		if err == nil {
			return ErrAlreadyExists
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		id := uuid.New()
		now := time.Now().UTC()
		var sess any
		if in.RequestedBySession != "" {
			if _, err := uuid.Parse(in.RequestedBySession); err == nil {
				sess = in.RequestedBySession
			}
		}
		_, err = tx.Exec(ctx, `
INSERT INTO approval_requests (
  id, org_id, action_code, resource_type, resource_id, branch_id, warehouse_id,
  payload, summary, status, requested_by_sub, requested_by_operator, requested_by_session, requested_at
) VALUES (
  $1, $2::uuid, $3, $4, $5, $6, $7,
  $8::jsonb, $9, 'PENDING', $10, $11, $12, $13
)`, id, orgID, in.ActionCode, in.ResourceType, in.ResourceID, in.BranchID, in.WarehouseID,
			string(raw), strings.TrimSpace(in.Summary), in.RequestedBySub, in.RequestedByOperator, sess, now)
		if err != nil {
			return err
		}
		out = Request{
			ID: id.String(), OrgID: orgID, ActionCode: in.ActionCode,
			ResourceType: in.ResourceType, ResourceID: in.ResourceID,
			BranchID: in.BranchID, WarehouseID: in.WarehouseID,
			Payload: raw, Summary: in.Summary, Status: "PENDING",
			RequestedBySub: in.RequestedBySub, RequestedByOperator: in.RequestedByOperator,
			RequestedBySession: in.RequestedBySession, RequestedAt: now,
		}
		return nil
	})
	return out, err
}

func (s *Store) ListPending(ctx context.Context, orgRef, branchCode string, limit int) ([]Request, error) {
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	var out []Request
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		q := `
SELECT id::text, org_id::text, action_code, resource_type, resource_id, branch_id, warehouse_id,
       payload, summary, status, requested_by_sub, requested_by_operator,
       COALESCE(requested_by_session::text,''), requested_at,
       COALESCE(decided_by_sub,''), COALESCE(decided_by_operator,''), decided_at,
       COALESCE(decision_reason,''), COALESCE(result_ref,'')
FROM approval_requests
WHERE org_id = $1::uuid AND status = 'PENDING'`
		args := []any{orgID}
		if branchCode != "" {
			q += ` AND (branch_id = $2 OR branch_id = '')`
			args = append(args, branchCode)
			q += ` ORDER BY requested_at ASC LIMIT $3`
			args = append(args, limit)
		} else {
			q += ` ORDER BY requested_at ASC LIMIT $2`
			args = append(args, limit)
		}
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			rec, err := scanRequest(rows)
			if err != nil {
				return err
			}
			out = append(out, rec)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) Get(ctx context.Context, orgRef, id string) (Request, error) {
	var out Request
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
SELECT id::text, org_id::text, action_code, resource_type, resource_id, branch_id, warehouse_id,
       payload, summary, status, requested_by_sub, requested_by_operator,
       COALESCE(requested_by_session::text,''), requested_at,
       COALESCE(decided_by_sub,''), COALESCE(decided_by_operator,''), decided_at,
       COALESCE(decision_reason,''), COALESCE(result_ref,'')
FROM approval_requests WHERE org_id = $1::uuid AND id = $2::uuid`, orgID, id)
		out, err = scanRequest(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	return out, err
}

func (s *Store) Decide(ctx context.Context, in DecideInput) (Request, error) {
	var out Request
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrg(ctx, tx, in.OrgRef)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
SELECT id::text, org_id::text, action_code, resource_type, resource_id, branch_id, warehouse_id,
       payload, summary, status, requested_by_sub, requested_by_operator,
       COALESCE(requested_by_session::text,''), requested_at,
       COALESCE(decided_by_sub,''), COALESCE(decided_by_operator,''), decided_at,
       COALESCE(decision_reason,''), COALESCE(result_ref,'')
FROM approval_requests WHERE org_id = $1::uuid AND id = $2::uuid
FOR UPDATE`, orgID, in.RequestID)
		cur, err := scanRequest(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cur.Status != "PENDING" {
			return ErrNotPending
		}
		status := "REJECTED"
		if in.Approve {
			status = "APPROVED"
		}
		now := time.Now().UTC()
		_, err = tx.Exec(ctx, `
UPDATE approval_requests SET
  status = $1, decided_by_sub = $2, decided_by_operator = $3, decided_at = $4,
  decision_reason = $5, result_ref = $6
WHERE id = $7::uuid`, status, in.DecidedBySub, in.DecidedByOperator, now, strings.TrimSpace(in.Reason), in.ResultRef, in.RequestID)
		if err != nil {
			return err
		}
		cur.Status = status
		cur.DecidedBySub = in.DecidedBySub
		cur.DecidedByOperator = in.DecidedByOperator
		cur.DecidedAt = &now
		cur.DecisionReason = in.Reason
		cur.ResultRef = in.ResultRef
		out = cur
		return nil
	})
	return out, err
}

type scanner interface{ Scan(dest ...any) error }

func scanRequest(row scanner) (Request, error) {
	var r Request
	var decidedAt *time.Time
	var payload []byte
	err := row.Scan(
		&r.ID, &r.OrgID, &r.ActionCode, &r.ResourceType, &r.ResourceID, &r.BranchID, &r.WarehouseID,
		&payload, &r.Summary, &r.Status, &r.RequestedBySub, &r.RequestedByOperator,
		&r.RequestedBySession, &r.RequestedAt,
		&r.DecidedBySub, &r.DecidedByOperator, &decidedAt,
		&r.DecisionReason, &r.ResultRef,
	)
	if err != nil {
		return Request{}, err
	}
	r.Payload = payload
	r.DecidedAt = decidedAt
	return r, nil
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
