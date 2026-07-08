package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
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

func (p *NATSPublisher) Publish(_ context.Context, env Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	subj := fmt.Sprintf("%s.%s", p.subject, env.Type)
	return p.nc.Publish(subj, body)
}

func (p *NATSPublisher) Close() error {
	p.nc.Close()
	return nil
}

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
