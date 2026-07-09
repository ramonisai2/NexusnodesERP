package deptmgr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

var (
	ErrUserNotFound       = errors.New("user_not_found")
	ErrBranchNotFound     = errors.New("branch_not_found")
	ErrDepartmentNotFound = errors.New("department_not_found")
)

type Store struct {
	Pool *pgxpool.Pool
}

type Assignment struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	UserSub        string     `json:"user_sub"`
	DisplayName    string     `json:"display_name"`
	Email          string     `json:"email,omitempty"`
	BranchID       string     `json:"branch_id"`
	BranchCode     string     `json:"branch_code"`
	DepartmentID   string     `json:"department_id"`
	DepartmentCode string     `json:"department_code"`
	DepartmentName string     `json:"department_name"`
	ValidFrom      time.Time  `json:"valid_from"`
	ValidTo        *time.Time `json:"valid_to,omitempty"`
}

type DepartmentOption struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type UserOption struct {
	ID          string `json:"id"`
	Sub         string `json:"sub"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
}

type ReplaceInput struct {
	OrgClaim         string
	ActorSub         string
	UserSub          string
	BranchCode       string
	DepartmentCodes  []string
}

// ListAssignments returns active department-manager rows for a branch (or all branches).
func (s *Store) ListAssignments(ctx context.Context, orgClaim, branchCode string) ([]Assignment, error) {
	var out []Assignment
	err := db.WithOrgTx(ctx, s.Pool, orgClaim, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgClaim)
		if err != nil {
			return err
		}
		q := `
SELECT dm.id::text, u.id::text, u.idp_sub, u.display_name, COALESCE(u.email, ''),
       b.id::text, b.code, sd.id::text, sd.code, sd.name, dm.valid_from, dm.valid_to
FROM department_managers dm
JOIN users u ON u.id = dm.user_id
JOIN store_departments sd ON sd.id = dm.department_id
JOIN branches b ON b.id = sd.branch_id
WHERE dm.org_id = $1::uuid
  AND (dm.valid_to IS NULL OR dm.valid_to > now())
  AND sd.active = TRUE`
		args := []any{orgID}
		if strings.TrimSpace(branchCode) != "" {
			q += ` AND b.code = $2`
			args = append(args, branchCode)
		}
		q += ` ORDER BY b.code, u.display_name, sd.sort_order, sd.code`
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a Assignment
			var validTo *time.Time
			if err := rows.Scan(
				&a.ID, &a.UserID, &a.UserSub, &a.DisplayName, &a.Email,
				&a.BranchID, &a.BranchCode, &a.DepartmentID, &a.DepartmentCode, &a.DepartmentName,
				&a.ValidFrom, &validTo,
			); err != nil {
				return err
			}
			a.ValidTo = validTo
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) ListDepartments(ctx context.Context, orgClaim, branchCode string) ([]DepartmentOption, error) {
	var out []DepartmentOption
	err := db.WithOrgTx(ctx, s.Pool, orgClaim, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgClaim)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
SELECT sd.id::text, sd.code, sd.name
FROM store_departments sd
JOIN branches b ON b.id = sd.branch_id
WHERE sd.org_id = $1::uuid AND b.code = $2 AND sd.active = TRUE
ORDER BY sd.sort_order, sd.code`, orgID, branchCode)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d DepartmentOption
			if err := rows.Scan(&d.ID, &d.Code, &d.Name); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) ListAssignableUsers(ctx context.Context, orgClaim string) ([]UserOption, error) {
	var out []UserOption
	err := db.WithOrgTx(ctx, s.Pool, orgClaim, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgClaim)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
SELECT DISTINCT u.id::text, u.idp_sub, u.display_name, COALESCE(u.email, '')
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN roles r ON r.id = ur.role_id
WHERE u.org_id = $1::uuid
  AND (ur.valid_to IS NULL OR ur.valid_to > now())
  AND r.code IN ('warehouse_manager', 'regional_manager', 'store_owner', 'platform_admin', 'inventory_clerk')
ORDER BY u.display_name`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var u UserOption
			if err := rows.Scan(&u.ID, &u.Sub, &u.DisplayName, &u.Email); err != nil {
				return err
			}
			out = append(out, u)
		}
		return rows.Err()
	})
	return out, err
}

// ReplaceAssignments sets the active department set for a user on a branch.
// Departments need not be related; any subset of the branch's departments is valid.
func (s *Store) ReplaceAssignments(ctx context.Context, in ReplaceInput) ([]Assignment, error) {
	in.UserSub = strings.TrimSpace(in.UserSub)
	in.BranchCode = strings.TrimSpace(in.BranchCode)
	if in.UserSub == "" || in.BranchCode == "" {
		return nil, fmt.Errorf("user_sub and branch_id required")
	}
	codes := uniqueNonEmpty(in.DepartmentCodes)

	err := db.WithOrgTx(ctx, s.Pool, in.OrgClaim, func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, in.OrgClaim)
		if err != nil {
			return err
		}
		var userID string
		err = tx.QueryRow(ctx, `
SELECT id::text FROM users WHERE org_id = $1::uuid AND idp_sub = $2 LIMIT 1`, orgID, in.UserSub).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		var branchID string
		err = tx.QueryRow(ctx, `
SELECT id::text FROM branches WHERE org_id = $1::uuid AND code = $2 LIMIT 1`, orgID, in.BranchCode).Scan(&branchID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBranchNotFound
		}
		if err != nil {
			return err
		}

		var actorID *string
		if strings.TrimSpace(in.ActorSub) != "" {
			var aid string
			if err := tx.QueryRow(ctx, `
SELECT id::text FROM users WHERE org_id = $1::uuid AND idp_sub = $2 LIMIT 1`, orgID, in.ActorSub).Scan(&aid); err == nil {
				actorID = &aid
			}
		}

		// Close current assignments for this user on this branch that are not in the new set.
		if _, err := tx.Exec(ctx, `
UPDATE department_managers dm
SET valid_to = now()
FROM store_departments sd
WHERE dm.department_id = sd.id
  AND dm.user_id = $1::uuid
  AND sd.branch_id = $2::uuid
  AND (dm.valid_to IS NULL OR dm.valid_to > now())
  AND NOT (sd.code = ANY($3::text[]))`, userID, branchID, codes); err != nil {
			return err
		}

		for _, code := range codes {
			var deptID string
			err := tx.QueryRow(ctx, `
SELECT id::text FROM store_departments
WHERE org_id = $1::uuid AND branch_id = $2::uuid AND code = $3 AND active = TRUE`,
				orgID, branchID, code).Scan(&deptID)
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: %s", ErrDepartmentNotFound, code)
			}
			if err != nil {
				return err
			}
			// Reactivate or insert.
			tag, err := tx.Exec(ctx, `
UPDATE department_managers
SET valid_to = NULL, assigned_by = COALESCE($3::uuid, assigned_by), valid_from = CASE WHEN valid_to IS NOT NULL THEN now() ELSE valid_from END
WHERE department_id = $1::uuid AND user_id = $2::uuid`, deptID, userID, actorID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				if _, err := tx.Exec(ctx, `
INSERT INTO department_managers (org_id, department_id, user_id, assigned_by)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid)
ON CONFLICT (department_id, user_id) DO UPDATE
SET valid_to = NULL, assigned_by = EXCLUDED.assigned_by, valid_from = now()`,
					orgID, deptID, userID, actorID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ListAssignments(ctx, in.OrgClaim, in.BranchCode)
}

func resolveOrgID(ctx context.Context, tx pgx.Tx, orgClaim string) (string, error) {
	if orgClaim == "" || orgClaim == "org_demo" {
		var id string
		err := tx.QueryRow(ctx, `SELECT id::text FROM organizations WHERE code = 'demo' LIMIT 1`).Scan(&id)
		if err == nil {
			return id, nil
		}
		// Fallback to known demo UUID from seeds.
		return "11111111-1111-1111-1111-111111111111", nil
	}
	var id string
	err := tx.QueryRow(ctx, `
SELECT id::text FROM organizations WHERE id::text = $1 OR code = $1 LIMIT 1`, orgClaim).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, c := range in {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}
