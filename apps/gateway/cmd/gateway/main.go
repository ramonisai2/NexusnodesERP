package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/approvals"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/auth"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/deptmgr"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/enrich"
	gwmw "github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/middleware"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/sessions"
	"github.com/ramonisai2/NexusnodesERP/apps/gateway/internal/setup"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
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
	var sessionStore *sessions.Store
	var approvalStore *approvals.Store
	var deptMgrStore *deptmgr.Store
	setupSvc := &setup.Service{}
	if os.Getenv("DATABASE_URL") != "" {
		pool, err := db.Connect(ctx)
		if err != nil {
			log.Fatalf("gateway db: %v", err)
		}
		enricher = enrich.New(pool)
		sessionStore = sessions.New(pool)
		approvalStore = approvals.New(pool)
		deptMgrStore = &deptmgr.Store{Pool: pool}
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
	messagingURL := mustURL(envOr("MESSAGING_URL", "http://localhost:8087"))

	r := chi.NewRouter()
	r.Use(otelx.Middleware("gateway"))
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))
	r.Use(gwmw.Hardening(1 << 20)) // security headers + body limit + JSON content-type
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{envOr("CORS_ORIGIN", "http://localhost:5173")},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept", "Authorization", "Content-Type", "Idempotency-Key", "X-Branch-Id",
			"X-Operator-Label", "X-Session-Id", "X-Station-Id",
		},
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

	// Public online storefront (no JWT). Path must not collide with /storefront/settings.
	r.Handle("GET /storefront/public/{slug}", reverseProxy(inventoryURL))

	// Customer self-registration / login (public) — mint customer JWT after inventory validates.
	r.Post("/customers/register", func(w http.ResponseWriter, req *http.Request) {
		handleCustomerAuth(w, req, inventoryURL, validator, http.MethodPost, "/customers/register", http.StatusCreated)
	})
	r.Post("/customers/login", func(w http.ResponseWriter, req *http.Request) {
		handleCustomerAuth(w, req, inventoryURL, validator, http.MethodPost, "/customers/login", http.StatusOK)
	})

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
		// Optional operator seat on shared username (concurrent stations).
		var station *sessions.Station
		opLabel := strings.TrimSpace(req.URL.Query().Get("operator_label"))
		if opLabel == "" {
			opLabel = strings.TrimSpace(req.Header.Get("X-Operator-Label"))
		}
		stationID := strings.TrimSpace(req.URL.Query().Get("station_id"))
		if stationID == "" {
			stationID = strings.TrimSpace(req.Header.Get("X-Station-Id"))
		}
		if opLabel != "" {
			effective.OperatorLabel = opLabel
			toIssue.OperatorLabel = opLabel
			if toIssue.Attrs == nil {
				toIssue.Attrs = map[string]any{}
			}
			if effective.Attrs == nil {
				effective.Attrs = map[string]any{}
			}
			toIssue.Attrs["operator_label"] = opLabel
			effective.Attrs["operator_label"] = opLabel
			if stationID != "" {
				toIssue.Attrs["station_id"] = stationID
				effective.Attrs["station_id"] = stationID
			}
			if sessionStore != nil {
				st, err := sessionStore.OpenStation(req.Context(), effective.OrgID, effective.Sub, opLabel, stationID, "", 8*time.Hour)
				if err == nil {
					station = &st
					effective.SessionID = st.ID
					toIssue.SessionID = st.ID
					toIssue.SID = st.ID
					effective.SID = st.ID
					toIssue.Attrs["session_id"] = st.ID
					effective.Attrs["session_id"] = st.ID
				} else {
					log.Printf("open station warning: %v", err)
				}
			}
		}

		token, err := validator.IssueDevToken(toIssue, 8*time.Hour)
		if err != nil {
			http.Error(w, `{"error":"token_issue_failed"}`, http.StatusInternalServerError)
			return
		}
		resp := map[string]any{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   28800,
			"claims":       effective,
			"persona":      personaOr(persona, "analyst"),
		}
		if station != nil {
			resp["station"] = station
		}
		writeJSON(w, http.StatusOK, resp)
	})

	r.Group(func(pr chi.Router) {
		pr.Use(validator.Middleware)
		pr.Use(enrichMiddleware(enricher))
		pr.Use(operatorHeadersMiddleware(sessionStore))

		pr.Get("/me", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			writeJSON(w, http.StatusOK, claims)
		})

		pr.Get("/me/effective-permissions", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			writeJSON(w, http.StatusOK, map[string]any{
				"permissions":    claims.Permissions,
				"roles":          claims.Roles,
				"branch_ids":     claims.BranchIDs,
				"attrs":          claims.Attrs,
				"operator_label": claims.OperatorLabel,
				"session_id":     claims.SessionID,
			})
		})

		// Concurrent operator stations under a shared account.
		pr.Get("/auth/stations", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			if sessionStore == nil {
				writeJSON(w, http.StatusOK, []any{})
				return
			}
			list, err := sessionStore.ListActive(req.Context(), claims.Sub)
			if err != nil {
				http.Error(w, `{"error":"stations_failed"}`, http.StatusInternalServerError)
				return
			}
			if list == nil {
				list = []sessions.Station{}
			}
			writeJSON(w, http.StatusOK, list)
		})
		pr.Post("/auth/operator-signin", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			var body struct {
				OperatorLabel string `json:"operator_label"`
				StationID     string `json:"station_id"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			opLabel := strings.TrimSpace(body.OperatorLabel)
			if opLabel == "" {
				http.Error(w, `{"error":"operator_label_required"}`, http.StatusBadRequest)
				return
			}
			claims.OperatorLabel = opLabel
			if claims.Attrs == nil {
				claims.Attrs = map[string]any{}
			}
			claims.Attrs["operator_label"] = opLabel
			if body.StationID != "" {
				claims.Attrs["station_id"] = strings.TrimSpace(body.StationID)
			}
			var station *sessions.Station
			if sessionStore != nil {
				st, err := sessionStore.OpenStation(req.Context(), claims.OrgID, claims.Sub, opLabel, body.StationID, "", 8*time.Hour)
				if err != nil {
					http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
					return
				}
				station = &st
				claims.SessionID = st.ID
				claims.SID = st.ID
				claims.Attrs["session_id"] = st.ID
			}
			token, err := validator.IssueDevToken(claims, 8*time.Hour)
			if err != nil {
				http.Error(w, `{"error":"token_issue_failed"}`, http.StatusInternalServerError)
				return
			}
			resp := map[string]any{
				"access_token": token,
				"token_type":   "Bearer",
				"expires_in":   28800,
				"claims":       claims,
			}
			if station != nil {
				resp["station"] = station
			}
			writeJSON(w, http.StatusOK, resp)
		})
		pr.Delete("/auth/stations/{id}", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			if sessionStore == nil {
				http.Error(w, `{"error":"database_required"}`, http.StatusServiceUnavailable)
				return
			}
			id := chi.URLParam(req, "id")
			if err := sessionStore.Revoke(req.Context(), id, claims.Sub); err != nil {
				if errors.Is(err, sessions.ErrNotFound) {
					http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
					return
				}
				http.Error(w, `{"error":"revoke_failed"}`, http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
		})

		// Department managers: one jefe may own several unrelated store departments.
		pr.Get("/org/department-managers", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			subject := claims.ToSubject()
			branchID := req.URL.Query().Get("branch_id")
			if branchID == "" && len(claims.BranchIDs) > 0 {
				branchID = claims.BranchIDs[0]
			}
			allow, err := opa.Allow(req.Context(), authz.Input{
				Subject:  subject,
				Action:   "store.department.manager.read",
				Resource: map[string]any{"branch_id": branchID, "org_id": claims.OrgID},
			})
			if err != nil {
				http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			if !allow {
				authz.WriteForbidden(w, "store.department.manager.read")
				return
			}
			if deptMgrStore == nil {
				http.Error(w, `{"error":"database_required"}`, http.StatusServiceUnavailable)
				return
			}
			list, err := deptMgrStore.ListAssignments(req.Context(), claims.OrgID, branchID)
			if err != nil {
				http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
				return
			}
			if list == nil {
				list = []deptmgr.Assignment{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": list, "branch_id": branchID})
		})

		pr.Get("/org/department-managers/options", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			subject := claims.ToSubject()
			branchID := req.URL.Query().Get("branch_id")
			if branchID == "" && len(claims.BranchIDs) > 0 {
				branchID = claims.BranchIDs[0]
			}
			allow, err := opa.Allow(req.Context(), authz.Input{
				Subject:  subject,
				Action:   "store.department.manager.read",
				Resource: map[string]any{"branch_id": branchID, "org_id": claims.OrgID},
			})
			if err != nil {
				http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			if !allow {
				authz.WriteForbidden(w, "store.department.manager.read")
				return
			}
			if deptMgrStore == nil {
				http.Error(w, `{"error":"database_required"}`, http.StatusServiceUnavailable)
				return
			}
			deps, err := deptMgrStore.ListDepartments(req.Context(), claims.OrgID, branchID)
			if err != nil {
				http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
				return
			}
			users, err := deptMgrStore.ListAssignableUsers(req.Context(), claims.OrgID)
			if err != nil {
				http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
				return
			}
			if deps == nil {
				deps = []deptmgr.DepartmentOption{}
			}
			if users == nil {
				users = []deptmgr.UserOption{}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"branch_id":   branchID,
				"departments": deps,
				"users":       users,
			})
		})

		pr.Put("/org/department-managers", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			subject := claims.ToSubject()
			var body struct {
				UserSub         string   `json:"user_sub"`
				BranchID        string   `json:"branch_id"`
				DepartmentCodes []string `json:"department_codes"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
				return
			}
			if body.BranchID == "" && len(claims.BranchIDs) > 0 {
				body.BranchID = claims.BranchIDs[0]
			}
			allow, err := opa.Allow(req.Context(), authz.Input{
				Subject:  subject,
				Action:   "store.department.manager.assign",
				Resource: map[string]any{"branch_id": body.BranchID, "org_id": claims.OrgID},
				Context:  map[string]any{"mfa_level": subject.MFALevel()},
			})
			if err != nil {
				http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			if !allow {
				authz.WriteForbidden(w, "store.department.manager.assign")
				return
			}
			if deptMgrStore == nil {
				http.Error(w, `{"error":"database_required"}`, http.StatusServiceUnavailable)
				return
			}
			list, err := deptMgrStore.ReplaceAssignments(req.Context(), deptmgr.ReplaceInput{
				OrgClaim:        claims.OrgID,
				ActorSub:        claims.Sub,
				UserSub:         body.UserSub,
				BranchCode:      body.BranchID,
				DepartmentCodes: body.DepartmentCodes,
			})
			if err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, deptmgr.ErrUserNotFound) || errors.Is(err, deptmgr.ErrBranchNotFound) || errors.Is(err, deptmgr.ErrDepartmentNotFound) {
					status = http.StatusNotFound
				}
				http.Error(w, fmt.Sprintf(`{"error":"assign_failed","detail":%q}`, err.Error()), status)
				return
			}
			if enricher != nil {
				enricher.Invalidate(body.UserSub, claims.OrgID)
			}
			if list == nil {
				list = []deptmgr.Assignment{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": list, "branch_id": body.BranchID, "user_sub": body.UserSub})
		})

		// Boss supervision queue (maker-checker).
		pr.Post("/approvals", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			subject := claims.ToSubject()
			applyOperatorFromRequest(req, &subject, &claims)
			if !subject.HasPermission("inventory.movement.void.request") &&
				!subject.HasPermission("approval.read") {
				authz.WriteForbidden(w, "inventory.movement.void.request")
				return
			}
			if approvalStore == nil {
				http.Error(w, `{"error":"database_required"}`, http.StatusServiceUnavailable)
				return
			}
			var body struct {
				ActionCode   string `json:"action_code"`
				ResourceType string `json:"resource_type"`
				ResourceID   string `json:"resource_id"`
				BranchID     string `json:"branch_id"`
				WarehouseID  string `json:"warehouse_id"`
				Summary      string `json:"summary"`
				Reason       string `json:"reason"`
				Payload      any    `json:"payload"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
				return
			}
			if body.ActionCode == "" {
				body.ActionCode = "inventory.movement.void"
			}
			if body.ResourceType == "" {
				body.ResourceType = "inventory_movement"
			}
			if body.ResourceID == "" {
				http.Error(w, `{"error":"resource_id_required"}`, http.StatusBadRequest)
				return
			}
			if body.ActionCode == "inventory.movement.void" {
				if !subject.HasPermission("inventory.movement.void.request") {
					authz.WriteForbidden(w, "inventory.movement.void.request")
					return
				}
				allow, err := opa.Allow(req.Context(), authz.Input{
					Subject: subject,
					Action:  "inventory.movement.void.request",
					Resource: map[string]any{
						"branch_id":    body.BranchID,
						"warehouse_id": body.WarehouseID,
						"org_id":       claims.OrgID,
						"movement_id":  body.ResourceID,
					},
				})
				if err != nil {
					http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
					return
				}
				if !allow {
					authz.WriteForbidden(w, "inventory.movement.void.request")
					return
				}
			}
			payload := body.Payload
			if payload == nil {
				payload = map[string]any{
					"reason":          body.Reason,
					"idempotency_key": req.Header.Get("Idempotency-Key"),
				}
			}
			summary := body.Summary
			if summary == "" {
				summary = fmt.Sprintf("Anular movimiento %s", body.ResourceID)
				if subject.OperatorLabel != "" {
					summary = fmt.Sprintf("%s (op: %s)", summary, subject.OperatorLabel)
				}
			}
			created, err := approvalStore.Create(req.Context(), approvals.CreateInput{
				OrgRef:              claims.OrgID,
				ActionCode:          body.ActionCode,
				ResourceType:        body.ResourceType,
				ResourceID:          body.ResourceID,
				BranchID:            body.BranchID,
				WarehouseID:         body.WarehouseID,
				Payload:             payload,
				Summary:             summary,
				RequestedBySub:      claims.Sub,
				RequestedByOperator: subject.OperatorLabel,
				RequestedBySession:  subject.SessionID,
			})
			if errors.Is(err, approvals.ErrAlreadyExists) {
				http.Error(w, `{"error":"already_pending"}`, http.StatusConflict)
				return
			}
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"create_failed","detail":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusCreated, created)
		})
		pr.Get("/approvals", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			subject := claims.ToSubject()
			if !subject.HasPermission("approval.read") && !subject.HasPermission("approval.decide") {
				authz.WriteForbidden(w, "approval.read")
				return
			}
			if approvalStore == nil {
				writeJSON(w, http.StatusOK, []any{})
				return
			}
			branch := req.URL.Query().Get("branch_id")
			if branch == "" {
				branch = req.Header.Get("X-Branch-Id")
			}
			list, err := approvalStore.ListPending(req.Context(), claims.OrgID, branch, 40)
			if err != nil {
				http.Error(w, `{"error":"list_failed"}`, http.StatusInternalServerError)
				return
			}
			if list == nil {
				list = []approvals.Request{}
			}
			writeJSON(w, http.StatusOK, list)
		})
		pr.Post("/approvals/{id}/decide", func(w http.ResponseWriter, req *http.Request) {
			claims, _ := auth.FromContext(req.Context())
			subject := claims.ToSubject()
			applyOperatorFromRequest(req, &subject, &claims)
			if !subject.HasPermission("approval.decide") {
				authz.WriteForbidden(w, "approval.decide")
				return
			}
			if approvalStore == nil {
				http.Error(w, `{"error":"database_required"}`, http.StatusServiceUnavailable)
				return
			}
			var body struct {
				Approve bool   `json:"approve"`
				Reason  string `json:"reason"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
				return
			}
			id := chi.URLParam(req, "id")
			pending, err := approvalStore.Get(req.Context(), claims.OrgID, id)
			if errors.Is(err, approvals.ErrNotFound) {
				http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, `{"error":"lookup_failed"}`, http.StatusInternalServerError)
				return
			}
			if pending.Status != "PENDING" {
				http.Error(w, `{"error":"not_pending"}`, http.StatusConflict)
				return
			}
			// Self-approval guard: boss cannot decide their own request under the same shared account seat.
			if pending.RequestedBySub == claims.Sub && pending.RequestedByOperator != "" &&
				pending.RequestedByOperator == subject.OperatorLabel {
				http.Error(w, `{"error":"self_approve_forbidden"}`, http.StatusForbidden)
				return
			}

			resultRef := ""
			if body.Approve && pending.ActionCode == "inventory.movement.void" {
				allow, err := opa.Allow(req.Context(), authz.Input{
					Subject: subject,
					Action:  "inventory.movement.void",
					Resource: map[string]any{
						"branch_id":    pending.BranchID,
						"warehouse_id": pending.WarehouseID,
						"org_id":       claims.OrgID,
						"movement_id":  pending.ResourceID,
					},
				})
				if err != nil {
					http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
					return
				}
				if !allow {
					authz.WriteForbidden(w, "inventory.movement.void")
					return
				}
				var payload map[string]any
				_ = json.Unmarshal(pending.Payload, &payload)
				reason, _ := payload["reason"].(string)
				if reason == "" {
					reason = body.Reason
				}
				if reason == "" {
					reason = "Aprobado por jefe"
				}
				idem, _ := payload["idempotency_key"].(string)
				if idem == "" {
					idem = "approval-" + pending.ID
				}
				voidBody, _ := json.Marshal(map[string]any{
					"org_id":          claims.OrgID,
					"reason":          reason,
					"idempotency_key": idem,
					"voided_by":       claims.Sub,
				})
				upURL := inventoryURL.String() + "/movements/" + pending.ResourceID + "/void"
				upReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, upURL, strings.NewReader(string(voidBody)))
				if err != nil {
					http.Error(w, `{"error":"proxy_build_failed"}`, http.StatusInternalServerError)
					return
				}
				upReq.Header.Set("Content-Type", "application/json")
				upReq.Header.Set("Idempotency-Key", idem)
				upReq.Header.Set("X-User-Id", claims.Sub)
				upReq.Header.Set("X-Org-Id", claims.OrgID)
				upReq.Header.Set("X-Branch-Ids", strings.Join(claims.BranchIDs, ","))
				upReq.Header.Set("X-Permissions", strings.Join(claims.Permissions, ","))
				upReq.Header.Set("X-Roles", strings.Join(claims.Roles, ","))
				upReq.Header.Set("X-Amr", strings.Join(claims.AMR, ","))
				upReq.Header.Set("X-Operator-Label", subject.OperatorLabel)
				upReq.Header.Set("X-Session-Id", subject.SessionID)
				if claims.Attrs != nil {
					if raw, err := json.Marshal(claims.Attrs); err == nil {
						upReq.Header.Set("X-Attrs-JSON", string(raw))
					}
				}
				upRes, err := http.DefaultClient.Do(upReq)
				if err != nil {
					http.Error(w, `{"error":"void_upstream_failed"}`, http.StatusBadGateway)
					return
				}
				defer upRes.Body.Close()
				if upRes.StatusCode >= 300 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(upRes.StatusCode)
					_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"void_failed","status":%d}`, upRes.StatusCode)))
					return
				}
				resultRef = pending.ResourceID
			}

			decided, err := approvalStore.Decide(req.Context(), approvals.DecideInput{
				OrgRef:            claims.OrgID,
				RequestID:         id,
				Approve:           body.Approve,
				DecidedBySub:      claims.Sub,
				DecidedByOperator: subject.OperatorLabel,
				Reason:            body.Reason,
				ResultRef:         resultRef,
			})
			if errors.Is(err, approvals.ErrNotPending) {
				http.Error(w, `{"error":"not_pending"}`, http.StatusConflict)
				return
			}
			if err != nil {
				http.Error(w, `{"error":"decide_failed"}`, http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, decided)
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
		pr.Handle("/mail/*", reverseProxy(messagingURL))
		pr.Handle("/mail", reverseProxy(messagingURL))
		pr.Handle("/storefront/*", reverseProxy(inventoryURL))
		pr.Handle("/storefront", reverseProxy(inventoryURL))
		pr.Handle("/customers/*", reverseProxy(inventoryURL))
		pr.Handle("/customers", reverseProxy(inventoryURL))
	})

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
			// Keep token AMR/SID/operator seat if enrichment did not set them.
			if len(enriched.AMR) == 0 {
				enriched.AMR = claims.AMR
			}
			if enriched.SID == "" {
				enriched.SID = claims.SID
			}
			if enriched.OperatorLabel == "" {
				enriched.OperatorLabel = claims.OperatorLabel
			}
			if enriched.SessionID == "" {
				enriched.SessionID = claims.SessionID
			}
			ctx := context.WithValue(r.Context(), auth.ClaimsContextKey, enriched)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// operatorHeadersMiddleware merges X-Operator-Label / X-Session-Id into claims and touches the station.
func operatorHeadersMiddleware(sessionStore *sessions.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.FromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			applyOperatorFromRequest(r, nil, &claims)
			if sessionStore != nil && claims.SessionID != "" {
				_ = sessionStore.Touch(r.Context(), claims.SessionID)
			}
			ctx := context.WithValue(r.Context(), auth.ClaimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func applyOperatorFromRequest(r *http.Request, subject *authz.Subject, claims *auth.Claims) {
	op := strings.TrimSpace(r.Header.Get("X-Operator-Label"))
	sid := strings.TrimSpace(r.Header.Get("X-Session-Id"))
	if claims != nil {
		if op == "" {
			op = claims.OperatorLabel
		}
		if sid == "" {
			sid = claims.SessionID
		}
		if op == "" && claims.Attrs != nil {
			if v, ok := claims.Attrs["operator_label"].(string); ok {
				op = v
			}
		}
		if sid == "" && claims.Attrs != nil {
			if v, ok := claims.Attrs["session_id"].(string); ok {
				sid = v
			}
		}
		if op != "" {
			claims.OperatorLabel = op
			if claims.Attrs == nil {
				claims.Attrs = map[string]any{}
			}
			claims.Attrs["operator_label"] = op
		}
		if sid != "" {
			claims.SessionID = sid
			if claims.Attrs == nil {
				claims.Attrs = map[string]any{}
			}
			claims.Attrs["session_id"] = sid
		}
	}
	if subject != nil {
		if op != "" {
			subject.OperatorLabel = op
		} else if claims != nil {
			subject.OperatorLabel = claims.OperatorLabel
		}
		if sid != "" {
			subject.SessionID = sid
		} else if claims != nil {
			subject.SessionID = claims.SessionID
		}
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
		case strings.HasPrefix(path, "/mail"):
			req.URL.Path = strings.TrimPrefix(path, "/mail")
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		case strings.HasPrefix(path, "/storefront"):
			// keep /storefront path on inventory (settings + public)
			req.URL.Path = path
		case strings.HasPrefix(path, "/customers"):
			// keep /customers path on inventory (accounts + cards)
			req.URL.Path = path
		}
		if claims, ok := auth.FromContext(req.Context()); ok {
			req.Header.Set("X-User-Id", claims.Sub)
			req.Header.Set("X-Org-Id", claims.OrgID)
			req.Header.Set("X-Branch-Ids", strings.Join(claims.BranchIDs, ","))
			req.Header.Set("X-Permissions", strings.Join(claims.Permissions, ","))
			req.Header.Set("X-Roles", strings.Join(claims.Roles, ","))
			req.Header.Set("X-Amr", strings.Join(claims.AMR, ","))
			if claims.OperatorLabel != "" {
				req.Header.Set("X-Operator-Label", claims.OperatorLabel)
			}
			if claims.SessionID != "" {
				req.Header.Set("X-Session-Id", claims.SessionID)
			} else if claims.SID != "" {
				req.Header.Set("X-Session-Id", claims.SID)
			}
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

func handleCustomerAuth(w http.ResponseWriter, req *http.Request, inventoryURL *url.URL, validator *auth.Validator, method, path string, okStatus int) {
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		http.Error(w, `{"error":"invalid_body"}`, http.StatusBadRequest)
		return
	}
	upURL := inventoryURL.String() + path
	upReq, err := http.NewRequestWithContext(req.Context(), method, upURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, `{"error":"proxy_build_failed"}`, http.StatusInternalServerError)
		return
	}
	upReq.Header.Set("Content-Type", "application/json")
	upRes, err := http.DefaultClient.Do(upReq)
	if err != nil {
		http.Error(w, `{"error":"upstream_failed"}`, http.StatusBadGateway)
		return
	}
	defer upRes.Body.Close()
	respBody, _ := io.ReadAll(upRes.Body)
	if upRes.StatusCode >= 300 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(upRes.StatusCode)
		_, _ = w.Write(respBody)
		return
	}
	var cust struct {
		ID                string `json:"id"`
		OrgID             string `json:"org_id"`
		Email             string `json:"email"`
		DisplayName       string `json:"display_name"`
		PreferredBranchID string `json:"preferred_branch_id"`
	}
	if err := json.Unmarshal(respBody, &cust); err != nil || cust.ID == "" {
		http.Error(w, `{"error":"invalid_upstream"}`, http.StatusBadGateway)
		return
	}
	claims := auth.CustomerClaims(cust.ID, cust.OrgID, cust.Email, cust.DisplayName, cust.PreferredBranchID)
	token, err := validator.IssueDevToken(claims, 24*time.Hour)
	if err != nil {
		http.Error(w, `{"error":"token_failed"}`, http.StatusInternalServerError)
		return
	}
	var customer any
	_ = json.Unmarshal(respBody, &customer)
	writeJSON(w, okStatus, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   86400,
		"claims":       claims,
		"customer":     customer,
	})
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
