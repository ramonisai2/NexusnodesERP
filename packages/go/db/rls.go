package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SetOrgContext sets request-scoped RLS GUC. Use inside a transaction (is_local=true).
func SetOrgContext(ctx context.Context, tx pgx.Tx, orgID string) error {
	_, err := tx.Exec(ctx, `SELECT set_config('app.org_id', $1, true)`, orgID)
	return err
}

// SetRLSBypass enables admin bypass for the current transaction.
func SetRLSBypass(ctx context.Context, tx pgx.Tx, on bool) error {
	val := "off"
	if on {
		val = "on"
	}
	_, err := tx.Exec(ctx, `SELECT set_config('app.rls_bypass', $1, true)`, val)
	return err
}

// WithOrgTx runs fn inside a transaction with app.org_id set for RLS.
func WithOrgTx(ctx context.Context, pool *pgxpool.Pool, orgID string, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if orgID != "" {
		if err := SetOrgContext(ctx, tx, orgID); err != nil {
			return err
		}
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// BeginOrgTx starts a transaction, resolves the org UUID with RLS bypass,
// then scopes the rest of the transaction to that org.
func BeginOrgTx(ctx context.Context, pool *pgxpool.Pool, resolveOrg func(pgx.Tx) (string, error)) (pgx.Tx, string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := SetRLSBypass(ctx, tx, true); err != nil {
		_ = tx.Rollback(ctx)
		return nil, "", err
	}
	orgID, err := resolveOrg(tx)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, "", err
	}
	if err := SetOrgContext(ctx, tx, orgID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, "", err
	}
	if err := SetRLSBypass(ctx, tx, false); err != nil {
		_ = tx.Rollback(ctx)
		return nil, "", err
	}
	return tx, orgID, nil
}
