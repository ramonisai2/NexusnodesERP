package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ctxKey string

const ClaimsContextKey ctxKey = "nexus_claims"

// Claims mirrors the access-token contract from the architecture docs.
type Claims struct {
	Sub         string         `json:"sub"`
	OrgID       string         `json:"org_id"`
	BranchIDs   []string       `json:"branch_ids"`
	Roles       []string       `json:"roles"`
	Permissions []string       `json:"permissions"`
	Attrs       map[string]any `json:"attrs"`
	AMR         []string       `json:"amr"`
	SID         string         `json:"sid"`
	jwt.RegisteredClaims
}

func (c Claims) HasPermission(code string) bool {
	for _, p := range c.Permissions {
		if p == code {
			return true
		}
	}
	return false
}

func (c Claims) HasBranch(branchID string) bool {
	for _, b := range c.BranchIDs {
		if b == branchID || b == "*" {
			return true
		}
	}
	return false
}

type Validator struct {
	secret   []byte
	issuer   string
	audience string
	bypass   bool
}

func NewValidatorFromEnv() *Validator {
	return &Validator{
		secret:   []byte(envOr("DEV_JWT_SECRET", "nexus-dev-secret-change-me")),
		issuer:   envOr("JWT_ISSUER", "http://localhost:8081/realms/nexus"),
		audience: envOr("JWT_AUDIENCE", "nexus-api"),
		bypass:   strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true"),
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (v *Validator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			if v.bypass {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ClaimsContextKey, DevClaims())))
				return
			}
			http.Error(w, `{"error":"missing_authorization"}`, http.StatusUnauthorized)
			return
		}
		raw := strings.TrimPrefix(header, "Bearer ")
		if raw == header {
			http.Error(w, `{"error":"invalid_authorization_scheme"}`, http.StatusUnauthorized)
			return
		}
		claims, err := v.Parse(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid_token","detail":%q}`, err.Error()), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ClaimsContextKey, claims)))
	})
}

func (v *Validator) Parse(tokenString string) (Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected alg %s", t.Method.Alg())
		}
		return v.secret, nil
	}, jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience))
	if err != nil {
		return Claims{}, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return Claims{}, errors.New("invalid claims")
	}
	return *claims, nil
}

// IssueDevToken mints an HS256 token for local development / tests.
func (v *Validator) IssueDevToken(claims Claims, ttl time.Duration) (string, error) {
	now := time.Now()
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    v.issuer,
		Audience:  jwt.ClaimStrings{v.audience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		ID:        uuid.NewString(),
		Subject:   claims.Sub,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(v.secret)
}

func DevClaims() Claims {
	return AnalystClaims()
}

func AnalystClaims() Claims {
	return Claims{
		Sub:       "usr_dev_analyst",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte", "br_sur"},
		Roles:     []string{"inventory_clerk", "payroll_analyst"},
		Permissions: []string{
			"inventory.movement.create",
			"inventory.balance.read",
			"payroll.run.prepare",
			"payroll.run.read",
		},
		Attrs: map[string]any{
			"max_payroll_amount": 1000000.0,
			"max_adjustment":     10000.0,
		},
		AMR: []string{"pwd", "otp"},
		SID: "sess_dev_analyst",
	}
}

func ApproverClaims() Claims {
	return Claims{
		Sub:       "usr_dev_approver",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte", "br_sur"},
		Roles:     []string{"payroll_approver"},
		Permissions: []string{
			"payroll.run.approve",
			"payroll.run.read",
			"inventory.balance.read",
		},
		Attrs: map[string]any{
			"max_payroll_amount": 1000000.0,
			"max_adjustment":     10000.0,
		},
		AMR: []string{"pwd", "otp"},
		SID: "sess_dev_approver",
	}
}

// DualClaims has prepare+approve to demonstrate SoD rejection when same subject does both.
func DualClaims() Claims {
	return Claims{
		Sub:       "usr_dev_dual",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte", "br_sur"},
		Roles:     []string{"payroll_analyst", "payroll_approver"},
		Permissions: []string{
			"payroll.run.prepare",
			"payroll.run.approve",
			"payroll.run.read",
		},
		Attrs: map[string]any{
			"max_payroll_amount": 1000000.0,
		},
		AMR: []string{"pwd", "otp"},
		SID: "sess_dev_dual",
	}
}

func FromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ClaimsContextKey).(Claims)
	return c, ok
}

// OPAClient evaluates RBAC+ABAC policies against Open Policy Agent.
type OPAClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewOPAClient(baseURL string) *OPAClient {
	return &OPAClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

type OPAInput struct {
	Subject  Claims         `json:"subject"`
	Action   string         `json:"action"`
	Resource map[string]any `json:"resource"`
	Context  map[string]any `json:"context"`
}

type opaRequest struct {
	Input OPAInput `json:"input"`
}

type opaResponse struct {
	Result struct {
		Allow bool `json:"allow"`
	} `json:"result"`
}

func (o *OPAClient) Allow(ctx context.Context, in OPAInput) (bool, error) {
	if o.baseURL == "" {
		// Fail-closed unless local bypass: permission must exist on token.
		return in.Subject.HasPermission(in.Action), nil
	}
	body, err := json.Marshal(opaRequest{Input: in})
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/data/nexus/authz", strings.NewReader(string(body)))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := o.httpClient.Do(req)
	if err != nil {
		// Soft-fail to token permissions when OPA is down in local/dev.
		if strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true") {
			return in.Subject.HasPermission(in.Action), nil
		}
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return false, fmt.Errorf("opa status %d", res.StatusCode)
	}
	var out opaResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Result.Allow, nil
}

func RequirePermission(opa *OPAClient, action string, resourceFromReq func(*http.Request) map[string]any) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok {
				http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
				return
			}
			resource := map[string]any{"org_id": claims.OrgID}
			if resourceFromReq != nil {
				for k, v := range resourceFromReq(r) {
					resource[k] = v
				}
			}
			allow, err := opa.Allow(r.Context(), OPAInput{
				Subject:  claims,
				Action:   action,
				Resource: resource,
				Context: map[string]any{
					"mfa_level": len(claims.AMR),
				},
			})
			if err != nil {
				http.Error(w, `{"error":"authz_unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			if !allow {
				http.Error(w, `{"error":"forbidden","action":"`+action+`"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
