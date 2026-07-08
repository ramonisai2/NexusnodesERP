package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type Envelope struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type Publisher interface {
	Publish(ctx context.Context, env Envelope) error
	Close() error
}

type NATSPublisher struct {
	nc      *nats.Conn
	subject string
}

func NewPublisherFromEnv() (Publisher, error) {
	url := os.Getenv("NATS_URL")
	if url == "" {
		return NewLogPublisher(), nil
	}
	nc, err := nats.Connect(url, nats.Name("nexus-outbox-relay"), nats.Timeout(3*time.Second))
	if err != nil {
		return nil, err
	}
	prefix := envOr("NATS_SUBJECT_PREFIX", "nexus.events")
	return &NATSPublisher{nc: nc, subject: strings.TrimRight(prefix, ".")}, nil
}

func (p *NATSPublisher) Publish(ctx context.Context, env Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	subj := fmt.Sprintf("%s.%s", p.subject, env.Type)
	msg := &nats.Msg{Subject: subj, Data: body, Header: nats.Header{}}
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(msg.Header))
	return p.nc.PublishMsg(msg)
}

func (p *NATSPublisher) Close() error {
	p.nc.Close()
	return nil
}

// natsHeaderCarrier adapts nats.Header to the OTel TextMapCarrier interface.
type natsHeaderCarrier nats.Header

func (c natsHeaderCarrier) Get(key string) string {
	vals := nats.Header(c).Values(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (c natsHeaderCarrier) Set(key, value string) {
	nats.Header(c).Set(key, value)
}

func (c natsHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

var _ propagation.TextMapCarrier = natsHeaderCarrier{}

type LogPublisher struct{}

func NewLogPublisher() *LogPublisher { return &LogPublisher{} }

func (p *LogPublisher) Publish(_ context.Context, env Envelope) error {
	fmt.Printf("event type=%s id=%s payload=%s\n", env.Type, env.ID, string(env.Payload))
	return nil
}

func (p *LogPublisher) Close() error { return nil }

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
