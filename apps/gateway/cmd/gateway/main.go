package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/auth"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/enrich"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/otelx"
)

func main() {
	ctx := context.Background()
	shutdown, err := otelx.Init(ctx, "gateway")
	if err != nil {
		log.Fatalf("otel: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	addr := envOr("GATEWAY_ADDR", ":8080")
	validator := auth.NewValidatorFromEnv()
	opa := auth.NewOPAClient(os.Getenv("OPA_URL"))

	var enricher *enrich.Enricher
	if os.Getenv("DATABASE_URL") != "" {
		pool, err := db.Connect(ctx)
		if err != nil {
			log.Fatalf("gateway db: %v", err)
		}
		enricher = enrich.New(pool)
		log.Printf("gateway claims enrichment=enabled")
	} else {
		log.Printf("gateway claims enrichment=disabled (no DATABASE_URL)")
	}

	inventoryURL := mustURL(envOr("INVENTORY_URL", "http://localhost:8082"))
	payrollURL := mustURL(envOr("PAYROLL_URL", "http://localhost:8083"))

	r := chi.NewRouter()
	r.Use(otelx.Middleware("gateway"))
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{envOr("CORS_ORIGIN", "http://localhost:5173")},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Idempotency-Key", "X-Branch-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "gateway"})
	})

	// Dev helper: mint a JWT for local SPA / curl without Keycloak.
	// persona=analyst|approver|dual (default analyst) to exercise payroll SoD.
	r.Post("/auth/dev-token", func(w http.ResponseWriter, req *http.Request) {
		if !strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true") {
			http.Error(w, `{"error":"disabled"}`, http.StatusForbidden)
			return
		}
		persona := req.URL.Query().Get("persona")
		claims := auth.AnalystClaims()
		switch persona {
		case "approver":
			claims = auth.ApproverClaims()
		case "dual":
			claims = auth.DualClaims()
		case "wh_manager":
			claims = auth.WarehouseManagerClaims()
		case "regional":
			claims = auth.RegionalManagerClaims()
		}
		// Issue a lean token; enrichment from DB happens on each authenticated request.
		lean := claims
		lean.Roles = nil
		lean.Permissions = nil
		lean.BranchIDs = nil
		lean.Attrs = map[string]any{}
		token, err := validator.IssueDevToken(lean, 8*time.Hour)
		if err != nil {
			http.Error(w, `{"error":"token_issue_failed"}`, http.StatusInternalServerError)
			return
		}
		effective := claims
		if enricher != nil {
			if enriched, err := enricher.Enrich(req.Context(), lean); err == nil {
				effective = enriched
				// Preserve AMR/SID from persona template when DB has no MFA attrs.
				if len(effective.AMR) == 0 {
					effective.AMR = claims.AMR
				}
				if effective.SID == "" {
					effective.SID = claims.SID
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   28800,
			"claims":       effective,
			"persona":      personaOr(persona, "analyst"),
		})
	})

	r.Group(func(pr chi.Router) {
		pr.Use(validator.Middleware)
		pr.Use(enrichMiddleware(enricher))

		pr.Get("/me", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			writeJSON(w, http.StatusOK, claims)
		})

		pr.Get("/me/effective-permissions", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			writeJSON(w, http.StatusOK, map[string]any{
				"permissions": claims.Permissions,
				"roles":       claims.Roles,
				"branch_ids":  claims.BranchIDs,
				"attrs":       claims.Attrs,
			})
		})

		pr.Handle("/inventory/*", reverseProxy(inventoryURL))
		pr.Handle("/inventory", reverseProxy(inventoryURL))
		pr.Handle("/payroll/*", reverseProxy(payrollURL))
		pr.Handle("/payroll", reverseProxy(payrollURL))
	})

	_ = opa

	log.Printf("gateway listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

func enrichMiddleware(enricher *enrich.Enricher) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.FromContext(r.Context())
			if !ok || enricher == nil {
				next.ServeHTTP(w, r)
				return
			}
			enriched, err := enricher.Enrich(r.Context(), claims)
			if err != nil {
				log.Printf("enrich warning: %v", err)
				next.ServeHTTP(w, r)
				return
			}
			// Keep token AMR/SID if enrichment did not set them.
			if len(enriched.AMR) == 0 {
				enriched.AMR = claims.AMR
			}
			if enriched.SID == "" {
				enriched.SID = claims.SID
			}
			ctx := context.WithValue(r.Context(), auth.ClaimsContextKey, enriched)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func reverseProxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	original := proxy.Director
	proxy.Director = func(req *http.Request) {
		original(req)
		req.Host = target.Host
		path := req.URL.Path
		switch {
		case strings.HasPrefix(path, "/inventory"):
			req.URL.Path = strings.TrimPrefix(path, "/inventory")
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		case strings.HasPrefix(path, "/payroll"):
			req.URL.Path = strings.TrimPrefix(path, "/payroll")
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}
		if claims, ok := auth.FromContext(req.Context()); ok {
			req.Header.Set("X-User-Id", claims.Sub)
			req.Header.Set("X-Org-Id", claims.OrgID)
			req.Header.Set("X-Branch-Ids", strings.Join(claims.BranchIDs, ","))
			req.Header.Set("X-Permissions", strings.Join(claims.Permissions, ","))
			req.Header.Set("X-Roles", strings.Join(claims.Roles, ","))
			req.Header.Set("X-Amr", strings.Join(claims.AMR, ","))
			if claims.Attrs != nil {
				if raw, err := json.Marshal(claims.Attrs); err == nil {
					req.Header.Set("X-Attrs-JSON", string(raw))
				}
			}
		}
	}
	return proxy
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

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("invalid url %s: %v", raw, err)
	}
	return u
}

func personaOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
