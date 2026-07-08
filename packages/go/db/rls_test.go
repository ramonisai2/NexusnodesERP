package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

func TestRLSOrgIsolation(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := db.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	var visible int
	err = db.WithOrgTx(context.Background(), pool, "11111111-1111-1111-1111-111111111111", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM branches`).Scan(&visible)
	})
	if err != nil {
		t.Fatalf("with org: %v", err)
	}
	if visible == 0 {
		t.Fatal("expected branches visible for demo org")
	}

	var hidden int
	err = db.WithOrgTx(context.Background(), pool, "00000000-0000-0000-0000-000000000099", func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM branches`).Scan(&hidden)
	})
	if err != nil {
		t.Fatalf("other org: %v", err)
	}
	if hidden != 0 {
		t.Fatalf("expected 0 branches for foreign org, got %d", hidden)
	}
}
