package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/graphql-go/graphql"
	"github.com/ramonisai2/NexusnodesERP/apps/reporting-bff/internal/graph"
	"github.com/ramonisai2/NexusnodesERP/apps/reporting-bff/internal/upstream"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
)

func main() {
	addr := envOr("REPORTING_ADDR", ":8084")
	ctx := context.Background()
	shutdown, err := otelx.Init(ctx, "reporting-bff")
	if err != nil {
		log.Fatalf("otel: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	svc := &graph.Services{
		Upstream: upstream.New(
			envOr("INVENTORY_URL", "http://127.0.0.1:8082"),
			envOr("PAYROLL_URL", "http://127.0.0.1:8083"),
		),
		OPA: authz.NewClientFromEnv(),
	}
	schema, err := graph.NewSchema(svc)
	if err != nil {
		log.Fatalf("schema: %v", err)
	}

	r := chi.NewRouter()
	r.Use(otelx.Middleware("reporting-bff"))
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(25 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "reporting-bff"})
	})

	r.Get("/graphql", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "reporting-bff",
			"hint":    "POST GraphQL queries to this endpoint",
		})
	})

	r.Post("/graphql", func(w http.ResponseWriter, req *http.Request) {
		subject := authz.FromGatewayHeaders(req)
		var body struct {
			Query         string         `json:"query"`
			OperationName string         `json:"operationName"`
			Variables     map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		if body.Query == "" {
			http.Error(w, `{"error":"query_required"}`, http.StatusBadRequest)
			return
		}
		ctx := context.WithValue(req.Context(), graph.SubjectContextKey(), subject)
		result := graphql.Do(graphql.Params{
			Schema:         schema,
			RequestString:  body.Query,
			VariableValues: body.Variables,
			OperationName: body.OperationName,
			Context:        ctx,
		})
		status := http.StatusOK
		if len(result.Errors) > 0 {
			for _, e := range result.Errors {
				if e.Message == "forbidden" {
					status = http.StatusForbidden
					break
				}
			}
		}
		writeJSON(w, status, result)
	})

	log.Printf("reporting-bff listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
