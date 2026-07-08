package otelx

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// Init configures a global TracerProvider.
// OTEL_EXPORTER=stdout|otlp|none (default none).
func Init(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	mode := strings.ToLower(envOr("OTEL_EXPORTER", "none"))
	if mode == "none" || mode == "off" || mode == "disabled" {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(envOr("SERVICE_VERSION", "0.1.0")),
		),
	)
	if err != nil {
		return nil, err
	}

	var spanExporter sdktrace.SpanExporter
	switch mode {
	case "stdout":
		spanExporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	case "otlp":
		opts := []otlptracehttp.Option{}
		if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); ep != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(ep), otlptracehttp.WithInsecure())
		}
		spanExporter, err = otlptracehttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("unsupported OTEL_EXPORTER=%s", mode)
	}
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

func Handler(service string, h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, service, otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		return r.Method + " " + r.URL.Path
	}))
}

func Middleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return Handler(service, next)
	}
}

// HTTPClient returns an http.Client that injects W3C trace context on outbound calls.
func HTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

// InjectHTTP injects the current span context into an outbound HTTP request.
func InjectHTTP(ctx context.Context, req *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
}

// ExtractHTTP extracts remote context from an inbound HTTP request.
func ExtractHTTP(ctx context.Context, req *http.Request) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(req.Header))
}

// HeaderCarrier adapts map[string]string for NATS / custom header propagation.
type HeaderCarrier map[string]string

func (c HeaderCarrier) Get(key string) string { return c[key] }
func (c HeaderCarrier) Set(key, value string) {
	if c == nil {
		return
	}
	c[key] = value
}
func (c HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectMap injects W3C trace context into a string map (e.g. NATS headers).
func InjectMap(ctx context.Context, headers map[string]string) {
	if headers == nil {
		return
	}
	otel.GetTextMapPropagator().Inject(ctx, HeaderCarrier(headers))
}

// ExtractMap extracts W3C trace context from a string map.
func ExtractMap(ctx context.Context, headers map[string]string) context.Context {
	if headers == nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, HeaderCarrier(headers))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
