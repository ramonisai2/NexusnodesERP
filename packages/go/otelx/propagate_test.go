package otelx

import (
	"context"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestInjectExtractHTTP(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	ctx, span := tp.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	req, err := http.NewRequest(http.MethodGet, "http://example.local/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	InjectHTTP(ctx, req)
	if req.Header.Get("traceparent") == "" {
		t.Fatal("expected traceparent")
	}
	out := ExtractHTTP(context.Background(), req)
	sc := trace.SpanContextFromContext(out)
	if !sc.IsValid() {
		t.Fatal("extracted span context invalid")
	}
	if sc.TraceID() != span.SpanContext().TraceID() {
		t.Fatalf("trace id mismatch")
	}
}

func TestInjectExtractMap(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))
	ctx, span := tp.Tracer("test").Start(context.Background(), "parent")
	defer span.End()
	h := map[string]string{}
	InjectMap(ctx, h)
	if h["traceparent"] == "" {
		t.Fatal("expected traceparent in map")
	}
	out := ExtractMap(context.Background(), h)
	sc := trace.SpanContextFromContext(out)
	if !sc.IsValid() || sc.TraceID() != span.SpanContext().TraceID() {
		t.Fatal("map extract failed")
	}
}
