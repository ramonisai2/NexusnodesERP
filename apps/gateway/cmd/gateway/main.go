package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/auth"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/enrich"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/setup"
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
	setupSvc := &setup.Service{}
	if os.Getenv("DATABASE_URL") != "" {
		pool, err := db.Connect(ctx)
		if err != nil {
			log.Fatalf("gateway db: %v", err)
		}
		enricher = enrich.New(pool)
		setupSvc.Pool = pool
		log.Printf("gateway claims enrichment=enabled")
	} else {
		log.Printf("gateway claims enrichment=disabled (no DATABASE_URL)")
	}

	inventoryURL := mustURL(envOr("INVENTORY_URL", "http://localhost:8082"))
	payrollURL := mustURL(envOr("PAYROLL_URL", "http://localhost:8083"))
	reportingURL := mustURL(envOr("REPORTING_URL", "http://localhost:8084"))
	reportsURL := mustURL(envOr("REPORTS_URL", "http://localhost:8085"))
	searchURL := mustURL(envOr("SEARCH_URL", "http://localhost:8086"))

	r := chi.NewRouter()
	r.Use(otelx.Middleware("gateway"))
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))
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

	// First-run installation panel (public): status + complete for small shops.
	r.Get("/setup/status", func(w http.ResponseWriter, req *http.Request) {
		st, err := setupSvc.Status(req.Context())
		if err != nil {
			http.Error(w, `{"error":"setup_status_failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	r.Get("/setup/presets", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, setup.Presets())
	})
	r.Post("/setup/complete", func(w http.ResponseWriter, req *http.Request) {
		var body setup.CompleteRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		result, err := setupSvc.Complete(req.Context(), body)
		if err != nil {
			msg := err.Error()
			status := http.StatusBadRequest
			switch msg {
			case "already_configured":
				status = http.StatusConflict
			case "database_required":
				status = http.StatusServiceUnavailable
			}
			http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), status)
			return
		}
		// Auto-login as the new store owner when DEV_AUTH_BYPASS is on.
		resp := map[string]any{"setup": result}
		if strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true") {
			claims := auth.StoreOwnerClaims(result.OwnerSub, result.OrgID, result.BranchCode, result.StoreName)
			token, err := validator.IssueDevToken(claims, 8*time.Hour)
			if err == nil {
				resp["access_token"] = token
				resp["token_type"] = "Bearer"
				resp["expires_in"] = 28800
				resp["claims"] = claims
			}
		}
		writeJSON(w, http.StatusCreated, resp)
	})

	// Public QR mobile upload (no JWT): phone opens /upload/{token} and POSTs files.
	r.Method(http.MethodGet, "/reports/images/upload/{token}", reverseProxy(reportsURL))
	r.Method(http.MethodPost, "/reports/images/upload/{token}", reverseProxy(reportsURL))

	// Dev helper: mint a JWT for local SPA / curl without Keycloak.
	// persona=analyst|approver|dual|owner (default analyst) to exercise payroll SoD.
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
		case "owner":
			claims = auth.StoreOwnerClaims("usr_dev_owner", "org_demo", "br_norte", "Mi Tienda")
			// Prefer the shop owner created by the setup wizard when present.
			if setupSvc != nil && setupSvc.Pool != nil {
				if sub, orgID, branch, store, ok := lookupInstalledOwner(req.Context(), setupSvc.Pool); ok {
					claims = auth.StoreOwnerClaims(sub, orgID, branch, store)
				}
			}
		}
		// Issue a lean token; enrichment from DB happens on each authenticated request.
		lean := claims
		lean.Roles = nil
		lean.Permissions = nil
		lean.BranchIDs = nil
		lean.Attrs = map[string]any{}
		toIssue := lean
		effective := claims
		if enricher != nil {
			if enriched, err := enricher.Enrich(req.Context(), lean); err == nil {
				if len(enriched.Permissions) > 0 {
					effective = enriched
					if len(effective.AMR) == 0 {
						effective.AMR = claims.AMR
					}
					if effective.SID == "" {
						effective.SID = claims.SID
					}
				} else {
					// No DB user for this persona (e.g. bare owner demo): embed template claims.
					toIssue = claims
					effective = claims
				}
			}
		} else {
			toIssue = claims
		}
		token, err := validator.IssueDevToken(toIssue, 8*time.Hour)
		if err != nil {
			http.Error(w, `{"error":"token_issue_failed"}`, http.StatusInternalServerError)
			return
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
		pr.Handle("/graphql", reverseProxy(reportingURL))
		pr.Handle("/graphql/*", reverseProxy(reportingURL))
		pr.Handle("/reports/*", reverseProxy(reportsURL))
		pr.Handle("/reports", reverseProxy(reportsURL))
		pr.Handle("/search/*", reverseProxy(searchURL))
		pr.Handle("/search", reverseProxy(searchURL))
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
		case path == "/graphql" || strings.HasPrefix(path, "/graphql/"):
			// keep /graphql path on reporting-bff
			req.URL.Path = path
		case strings.HasPrefix(path, "/reports"):
			req.URL.Path = strings.TrimPrefix(path, "/reports")
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		case strings.HasPrefix(path, "/search"):
			req.URL.Path = strings.TrimPrefix(path, "/search")
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
		// Propagate W3C trace context to upstream services.
		otelx.InjectHTTP(req.Context(), req)
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

// lookupInstalledOwner returns the shop owner created by the setup wizard, if any.
func lookupInstalledOwner(ctx context.Context, pool *pgxpool.Pool) (sub, orgID, branch, store string, ok bool) {
	if pool == nil {
		return "", "", "", "", false
	}
	err := db.WithOrgTx(ctx, pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
SELECT owner_sub, org_id::text, branch_code, store_name
FROM app_install WHERE id = 1 AND completed_at IS NOT NULL`).Scan(&sub, &orgID, &branch, &store)
	})
	if err != nil {
		return "", "", "", "", false
	}
	if sub == "" || orgID == "" {
		return "", "", "", "", false
	}
	return sub, orgID, branch, store, true
}
