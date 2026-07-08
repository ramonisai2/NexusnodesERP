package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/events"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	pub, err := events.NewPublisherFromEnv()
	if err != nil {
		log.Fatalf("publisher: %v", err)
	}
	defer pub.Close()

	interval := 2 * time.Second
	if v := os.Getenv("OUTBOX_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}
	batch := 50
	log.Printf("outbox-relay started interval=%s nats=%s", interval, os.Getenv("NATS_URL"))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := publishBatch(ctx, pool, pub, batch); err != nil {
			log.Printf("batch error: %v", err)
		}
		select {
		case <-ctx.Done():
			log.Printf("shutting down")
			return
		case <-ticker.C:
		}
	}
}

func publishBatch(ctx context.Context, pool *pgxpool.Pool, pub events.Publisher, limit int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
SELECT id::text, event_type, payload, created_at
FROM outbox
WHERE published_at IS NULL
ORDER BY created_at
FOR UPDATE SKIP LOCKED
LIMIT $1`, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	type row struct {
		id        string
		eventType string
		payload   []byte
		createdAt time.Time
	}
	var batch []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.eventType, &r.payload, &r.createdAt); err != nil {
			return err
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(batch) == 0 {
		return tx.Commit(ctx)
	}

	for _, r := range batch {
		env := events.Envelope{
			ID:        r.id,
			Type:      r.eventType,
			Payload:   json.RawMessage(r.payload),
			CreatedAt: r.createdAt,
		}
		if err := pub.Publish(ctx, env); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = $1::uuid`, r.id); err != nil {
			return err
		}
	}
	log.Printf("published %d outbox events", len(batch))
	return tx.Commit(ctx)
}
