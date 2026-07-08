package events_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ramonisai2/NexusnodesERP/packages/go/events"
)

func TestLogPublisher(t *testing.T) {
	p := events.NewLogPublisher()
	err := p.Publish(context.Background(), events.Envelope{
		ID:        "1",
		Type:      "InventoryMoved",
		Payload:   json.RawMessage(`{"ok":true}`),
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
}
