package otelx_test

import (
	"context"
	"testing"

	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
)

func TestInitNone(t *testing.T) {
	t.Setenv("OTEL_EXPORTER", "none")
	shutdown, err := otelx.Init(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
