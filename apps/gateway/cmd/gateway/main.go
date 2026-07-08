package main

import (
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
)

func main() {
	addr := envOr("GATEWAY_ADDR", ":8080")
	validator := auth.NewValidatorFromEnv()
	opa := auth.NewOPAClient(os.Getenv("OPA_URL"))

	inventoryURL := mustURL(envOr("INVENTORY_URL", "http://localhost:8082"))
	payrollURL := mustURL(envOr("PAYROLL_URL", "http://localhost:8083"))

	r := chi.NewRouter()
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
	// persona=analyst|approver (default analyst) to exercise payroll SoD.
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
		}
		token, err := validator.IssueDevToken(claims, 8*time.Hour)
		if err != nil {
			http.Error(w, `{"error":"token_issue_failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   28800,
			"claims":       claims,
			"persona":      personaOr(persona, "analyst"),
		})
	})

	r.Group(func(pr chi.Router) {
		pr.Use(validator.Middleware)

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

		// Proxied domain APIs — JWT already validated; downstream re-checks ABAC.
		pr.Handle("/inventory/*", reverseProxy(inventoryURL))
		pr.Handle("/inventory", reverseProxy(inventoryURL))
		pr.Handle("/payroll/*", reverseProxy(payrollURL))
		pr.Handle("/payroll", reverseProxy(payrollURL))
	})

	_ = opa // wired in downstream services; gateway keeps client for future coarse checks

	log.Printf("gateway listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

func reverseProxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	original := proxy.Director
	proxy.Director = func(req *http.Request) {
		original(req)
		req.Host = target.Host
		// Strip /inventory or /payroll prefix is NOT done — services listen on root paths
		// matching the gateway mount via path rewrite below.
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
