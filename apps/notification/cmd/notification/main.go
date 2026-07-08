package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/events"
	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const consumerName = "notification"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	shutdown, err := otelx.Init(ctx, "notification")
	if err != nil {
		log.Fatalf("otel: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	natsURL := envOr("NATS_URL", "nats://127.0.0.1:4222")
	nc, err := nats.Connect(natsURL, nats.Name("nexus-notification"), nats.Timeout(3*time.Second))
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	prefix := strings.TrimRight(envOr("NATS_SUBJECT_PREFIX", "nexus.events"), ".")
	subjects := []string{
		prefix + ".InventoryMoved",
		prefix + ".PayrollRunPrepared",
		prefix + ".PayrollRunApproved",
	}

	for _, subj := range subjects {
		subj := subj
		_, err := nc.Subscribe(subj, func(msg *nats.Msg) {
			handleMsg(pool, msg)
		})
		if err != nil {
			log.Fatalf("subscribe %s: %v", subj, err)
		}
		log.Printf("subscribed %s", subj)
	}

	log.Printf("notification consumer ready")
	<-ctx.Done()
	log.Printf("shutting down")
}

func handleMsg(pool *pgxpool.Pool, msg *nats.Msg) {
	tracer := otelx.Tracer("notification")
	ctx, span := tracer.Start(context.Background(), "notification.handle")
	defer span.End()
	span.SetAttributes(attribute.String("nats.subject", msg.Subject))

	var env events.Envelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "decode")
		log.Printf("decode error: %v", err)
		return
	}
	span.SetAttributes(
		attribute.String("event.id", env.ID),
		attribute.String("event.type", env.Type),
	)

	if err := processEvent(ctx, pool, env); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		log.Printf("process %s/%s: %v", env.Type, env.ID, err)
		return
	}
	log.Printf("processed %s id=%s", env.Type, env.ID)
}

func processEvent(ctx context.Context, pool *pgxpool.Pool, env events.Envelope) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	subject, body, orgID := renderNotification(env)
	if orgID != "" {
		if err := db.SetOrgContext(ctx, tx, orgID); err != nil {
			return err
		}
	} else if err := db.SetRLSBypass(ctx, tx, true); err != nil {
		return err
	}

	// Idempotency
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1::uuid AND consumer = $2)`,
		env.ID, consumerName).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit(ctx)
	}

	var orgArg any
	if orgID != "" {
		orgArg = orgID
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO notifications (org_id, channel, event_type, subject, body, payload, status)
VALUES ($1, 'log', $2, $3, $4, $5::jsonb, 'SENT')`,
		orgArg, env.Type, subject, body, string(env.Payload)); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO audit_log (org_id, action, resource_type, resource_id, before_after)
VALUES ($1, $2, 'event', $3::uuid, jsonb_build_object('consumer', $4::text, 'payload', $5::jsonb))`,
		orgArg, "event."+env.Type, env.ID, consumerName, string(env.Payload)); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO processed_events (event_id, consumer, event_type)
VALUES ($1::uuid, $2, $3)`, env.ID, consumerName, env.Type); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func renderNotification(env events.Envelope) (subject, body, orgID string) {
	var payload map[string]any
	_ = json.Unmarshal(env.Payload, &payload)
	if v, ok := payload["org_id"].(string); ok {
		orgID = resolveOrgUUID(v)
	}
	if orgID == "" {
		orgID = "11111111-1111-1111-1111-111111111111"
	}

	switch env.Type {
	case "InventoryMoved":
		subject = "Inventario: movimiento registrado"
		body = "Se publicó un movimiento de inventario."
		if sku, ok := payload["sku_id"].(string); ok {
			body = "Movimiento de SKU " + sku
		}
	case "PayrollRunPrepared":
		subject = "Nómina: corrida preparada"
		body = "Una corrida de nómina está lista para revisión."
	case "PayrollRunApproved":
		subject = "Nómina: corrida aprobada"
		body = "Una corrida de nómina fue aprobada."
	default:
		subject = "Evento " + env.Type
		body = "Evento de dominio recibido."
	}
	return subject, body, orgID
}

func resolveOrgUUID(ref string) string {
	if ref == "org_demo" || ref == "DEMO" {
		return "11111111-1111-1111-1111-111111111111"
	}
	if _, err := uuid.Parse(ref); err == nil {
		return ref
	}
	return ""
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
