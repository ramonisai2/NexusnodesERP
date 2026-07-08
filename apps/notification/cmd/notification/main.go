package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/ramonisai2/NexusnodesERP/apps/notification/internal/mail"
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

	sender := newMailSender()
	recipients := mail.ParseRecipients(envOr("NOTIFY_EMAIL_TO", "ops@demo.nexus,analyst@demo.nexus"))

	natsURL := envOr("NATS_URL", "nats://127.0.0.1:4222")
	nc, err := nats.Connect(natsURL, nats.Name("nexus-notification"), nats.Timeout(3*time.Second))
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	prefix := strings.TrimRight(envOr("NATS_SUBJECT_PREFIX", "nexus.events"), ".")
	subjects := []string{
		prefix + ".InventoryMoved",
		prefix + ".InventoryMovedVoided",
		prefix + ".PayrollRunPrepared",
		prefix + ".PayrollRunApproved",
	}

	queue := envOr("NATS_QUEUE_GROUP", "notification")
	for _, subj := range subjects {
		subj := subj
		_, err := nc.QueueSubscribe(subj, queue, func(msg *nats.Msg) {
			handleMsg(pool, sender, recipients, msg)
		})
		if err != nil {
			log.Fatalf("subscribe %s: %v", subj, err)
		}
		log.Printf("subscribed %s (queue=%s)", subj, queue)
	}

	log.Printf("notification consumer ready (email=%T to=%v)", sender, recipients)
	<-ctx.Done()
	log.Printf("shutting down")
}

func newMailSender() mail.Sender {
	if !strings.EqualFold(envOr("SMTP_ENABLED", "true"), "true") {
		log.Printf("SMTP disabled; using log sender")
		return mail.NewLogSender()
	}
	host := envOr("SMTP_HOST", "127.0.0.1")
	port := envOr("SMTP_PORT", "1025")
	from := envOr("SMTP_FROM", "nexus@demo.local")
	log.Printf("SMTP enabled host=%s port=%s from=%s", host, port, from)
	return mail.NewSMTPSender(host, port, from)
}

func handleMsg(pool *pgxpool.Pool, sender mail.Sender, recipients []string, msg *nats.Msg) {
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

	if err := processEvent(ctx, pool, sender, recipients, env); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		log.Printf("process %s/%s: %v", env.Type, env.ID, err)
		return
	}
	log.Printf("processed %s id=%s", env.Type, env.ID)
}

func processEvent(ctx context.Context, pool *pgxpool.Pool, sender mail.Sender, recipients []string, env events.Envelope) error {
	subject, summary, details, orgID := renderNotification(env)
	text, htmlBody := mail.Render(mail.TemplateInput{
		EventType: env.Type,
		Title:     subject,
		Summary:   summary,
		Details:   details,
	})

	claimed, err := claimEvent(ctx, pool, env)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	channel := "log"
	status := "SENT"
	if len(recipients) > 0 {
		if err := sender.Send(mail.Message{
			To:      recipients,
			Subject: subject,
			Text:    text,
			HTML:    htmlBody,
		}); err != nil {
			status = "FAILED"
			log.Printf("email send failed: %v", err)
		} else {
			channel = "email"
		}
	}

	return persistNotification(ctx, pool, env, orgID, channel, status, subject, text, recipients)
}

// claimEvent inserts processed_events for this consumer. Returns false if already claimed.
func claimEvent(ctx context.Context, pool *pgxpool.Pool, env events.Envelope) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var claimedID string
	err = tx.QueryRow(ctx, `
INSERT INTO processed_events (event_id, consumer, event_type)
VALUES ($1::uuid, $2, $3)
ON CONFLICT (event_id, consumer) DO NOTHING
RETURNING event_id::text`, env.ID, consumerName, env.Type).Scan(&claimedID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func persistNotification(ctx context.Context, pool *pgxpool.Pool, env events.Envelope, orgID, channel, status, subject, body string, recipients []string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if orgID != "" {
		if err := db.SetOrgContext(ctx, tx, orgID); err != nil {
			return err
		}
	} else if err := db.SetRLSBypass(ctx, tx, true); err != nil {
		return err
	}

	var orgArg any
	if orgID != "" {
		orgArg = orgID
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO notifications (org_id, channel, event_type, subject, body, payload, status)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)`,
		orgArg, channel, env.Type, subject, body, string(env.Payload), status); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO audit_log (org_id, action, resource_type, resource_id, before_after)
VALUES ($1, $2, 'event', $3::uuid, jsonb_build_object(
  'consumer', $4::text,
  'channel', $5::text,
  'status', $6::text,
  'recipients', $7::jsonb,
  'payload', $8::jsonb
))`,
		orgArg, "event."+env.Type, env.ID, consumerName, channel, status,
		mustJSON(recipients), string(env.Payload)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func renderNotification(env events.Envelope) (subject, summary string, details map[string]string, orgID string) {
	var payload map[string]any
	_ = json.Unmarshal(env.Payload, &payload)
	if v, ok := payload["org_id"].(string); ok {
		orgID = resolveOrgUUID(v)
	}
	if orgID == "" {
		orgID = "11111111-1111-1111-1111-111111111111"
	}
	details = map[string]string{}
	add := func(k, path string) {
		if v, ok := asString(payload[path]); ok && v != "" {
			details[k] = v
		}
	}

	switch env.Type {
	case "InventoryMoved":
		subject = "Inventario: movimiento registrado"
		summary = "Se registró un movimiento de inventario en NexusERP."
		add("SKU", "sku_id")
		add("Almacén", "warehouse_id")
		add("Sucursal", "branch_id")
		add("Cantidad", "quantity")
		add("Movimiento", "movement_id")
	case "InventoryMovedVoided":
		subject = "Inventario: movimiento revertido"
		summary = "Un jefe de área revirtió un movimiento (error de captura / mala práctica)."
		add("Movimiento", "movement_id")
		add("Compensación", "compensation_id")
		add("SKU", "sku_id")
		add("Almacén", "warehouse_id")
		add("Sucursal", "branch_id")
		add("Motivo", "reason")
		add("Revertido por", "voided_by")
	case "PayrollRunPrepared":
		subject = "Nómina: corrida preparada"
		summary = "Una corrida de nómina está lista para revisión y aprobación."
		add("Corrida", "run_id")
		add("Periodo", "period_label")
		add("Sucursal", "branch_id")
		add("Preparó", "prepared_by")
		add("Total", "total_amount")
	case "PayrollRunApproved":
		subject = "Nómina: corrida aprobada"
		summary = "Una corrida de nómina fue aprobada."
		add("Corrida", "run_id")
		add("Periodo", "period_label")
		add("Sucursal", "branch_id")
		add("Aprobó", "approved_by")
		add("Total", "total_amount")
	default:
		subject = "Evento " + env.Type
		summary = "Evento de dominio recibido por el canal de notificaciones."
	}
	return subject, summary, details, orgID
}

func asString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t)), true
		}
		return fmt.Sprintf("%v", t), true
	case json.Number:
		return t.String(), true
	default:
		if v == nil {
			return "", false
		}
		return fmt.Sprintf("%v", v), true
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
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
