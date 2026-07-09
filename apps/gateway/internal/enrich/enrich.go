package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/auth"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

type Enricher struct {
	pool *pgxpool.Pool

	mu    sync.Mutex
	cache map[string]cacheEntry
	ttl   time.Duration
}

type cacheEntry struct {
	claims    auth.Claims
	expiresAt time.Time
}

func New(pool *pgxpool.Pool) *Enricher {
	return &Enricher{
		pool:  pool,
		cache: map[string]cacheEntry{},
		ttl:   30 * time.Second,
	}
}

// Enrich merges DB-backed roles, permissions, branches and attributes into JWT claims.
func (e *Enricher) Enrich(ctx context.Context, in auth.Claims) (auth.Claims, error) {
	if e == nil || e.pool == nil || in.Sub == "" {
		return in, nil
	}
	key := in.Sub + "|" + in.OrgID
	e.mu.Lock()
	if c, ok := e.cache[key]; ok && time.Now().Before(c.expiresAt) {
		out := c.claims
		e.mu.Unlock()
		return out, nil
	}
	e.mu.Unlock()

	out := in
	err := db.WithOrgTx(ctx, e.pool, "", func(tx pgx.Tx) error {
		// Enrichment needs to see the user row; use bypass for lookup then scope.
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		var userID, orgID string
		err := tx.QueryRow(ctx, `
SELECT id::text, org_id::text FROM users
WHERE idp_sub = $1
ORDER BY created_at
LIMIT 1`, in.Sub).Scan(&userID, &orgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out.OrgID = mapOrgClaim(orgID, in.OrgID)

		roles, err := loadRoles(ctx, tx, userID)
		if err != nil {
			return err
		}
		if len(roles) > 0 {
			out.Roles = roles
		}
		perms, err := loadPermissions(ctx, tx, userID)
		if err != nil {
			return err
		}
		if len(perms) > 0 {
			out.Permissions = perms
		}
		branches, err := loadBranches(ctx, tx, userID, orgID)
		if err != nil {
			return err
		}
		if len(branches) > 0 {
			out.BranchIDs = branches
		}
		attrs, err := loadAttrs(ctx, tx, userID)
		if err != nil {
			return err
		}
		if out.Attrs == nil {
			out.Attrs = map[string]any{}
		}
		for k, v := range attrs {
			out.Attrs[k] = v
		}
		managed, err := loadManagedWarehouses(ctx, tx, userID)
		if err != nil {
			return err
		}
		if len(managed) > 0 {
			out.Attrs["managed_warehouses"] = managed
		}
		depts, err := loadManagedDepartments(ctx, tx, userID)
		if err != nil {
			return err
		}
		if len(depts) > 0 {
			out.Attrs["managed_departments"] = depts
		}
		mods, err := loadEnabledModules(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if mods != nil {
			out.Attrs["enabled_modules"] = mods
		}
		return nil
	})
	if err != nil {
		return in, err
	}

	e.mu.Lock()
	e.cache[key] = cacheEntry{claims: out, expiresAt: time.Now().Add(e.ttl)}
	e.mu.Unlock()
	return out, nil
}

// Invalidate clears cached claims for a subject (call after manager assignment changes).
func (e *Enricher) Invalidate(sub, orgID string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.cache, sub+"|"+orgID)
	// Also drop entries keyed with empty org if present.
	delete(e.cache, sub+"|")
}

func mapOrgClaim(orgUUID, fallback string) string {
	// Keep stable public claim used by SPA/dev tokens when org is demo.
	if orgUUID == "11111111-1111-1111-1111-111111111111" {
		return "org_demo"
	}
	if fallback != "" {
		return fallback
	}
	return orgUUID
}

func loadRoles(ctx context.Context, tx pgx.Tx, userID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
SELECT DISTINCT r.code
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = $1::uuid
  AND (ur.valid_to IS NULL OR ur.valid_to > now())`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func loadPermissions(ctx context.Context, tx pgx.Tx, userID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
SELECT DISTINCT p.code
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ur.user_id = $1::uuid
  AND (ur.valid_to IS NULL OR ur.valid_to > now())`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func loadBranches(ctx context.Context, tx pgx.Tx, userID, orgID string) ([]string, error) {
	// If any org-wide role (branch_id NULL), return all active branches.
	var orgWide bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1 FROM user_roles
  WHERE user_id = $1::uuid AND branch_id IS NULL
    AND (valid_to IS NULL OR valid_to > now())
)`, userID).Scan(&orgWide); err != nil {
		return nil, err
	}
	if orgWide {
		rows, err := tx.Query(ctx, `SELECT code FROM branches WHERE org_id = $1::uuid AND active = TRUE ORDER BY code`, orgID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var code string
			if err := rows.Scan(&code); err != nil {
				return nil, err
			}
			out = append(out, code)
		}
		return out, rows.Err()
	}
	rows, err := tx.Query(ctx, `
SELECT DISTINCT b.code
FROM user_roles ur
JOIN branches b ON b.id = ur.branch_id
WHERE ur.user_id = $1::uuid
  AND (ur.valid_to IS NULL OR ur.valid_to > now())`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func loadAttrs(ctx context.Context, tx pgx.Tx, userID string) (map[string]any, error) {
	rows, err := tx.Query(ctx, `
SELECT attr_key, attr_value FROM user_attributes WHERE user_id = $1::uuid`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			out[key] = string(raw)
			continue
		}
		out[key] = v
	}
	return out, rows.Err()
}

// loadManagedWarehouses resolves warehouses under org units the user manages,
// including descendant areas (regional manager → child areas → warehouses).
func loadManagedWarehouses(ctx context.Context, tx pgx.Tx, userID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
WITH RECURSIVE managed AS (
  SELECT ou.id
  FROM org_unit_managers oum
  JOIN org_units ou ON ou.id = oum.org_unit_id
  WHERE oum.user_id = $1::uuid
    AND (oum.valid_to IS NULL OR oum.valid_to > now())
    AND ou.active = TRUE
  UNION
  SELECT child.id
  FROM org_units child
  JOIN managed m ON child.parent_id = m.id
  WHERE child.active = TRUE
)
SELECT DISTINCT w.code
FROM warehouses w
JOIN managed m ON w.org_unit_id = m.id
ORDER BY w.code`, userID)
	if err != nil {
		// Hierarchy tables may not exist yet in older DBs.
		if strings.Contains(err.Error(), "org_unit") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

// loadManagedDepartments returns store department codes the user manages.
// Multiple unrelated departments are allowed (no parent/child requirement).
func loadManagedDepartments(ctx context.Context, tx pgx.Tx, userID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
SELECT DISTINCT sd.code
FROM department_managers dm
JOIN store_departments sd ON sd.id = dm.department_id
WHERE dm.user_id = $1::uuid
  AND (dm.valid_to IS NULL OR dm.valid_to > now())
  AND sd.active = TRUE
ORDER BY sd.code`, userID)
	if err != nil {
		if strings.Contains(err.Error(), "department_managers") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func loadEnabledModules(ctx context.Context, tx pgx.Tx, orgID string) ([]string, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `
SELECT enabled_modules FROM organizations WHERE id = $1::uuid`, orgID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		if strings.Contains(err.Error(), "enabled_modules") {
			return nil, nil
		}
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var mods []string
	if err := json.Unmarshal(raw, &mods); err != nil {
		return nil, nil
	}
	return mods, nil
}
