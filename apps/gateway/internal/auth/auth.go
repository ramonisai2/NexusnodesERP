package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
)

type ctxKey string

const ClaimsContextKey ctxKey = "nexus_claims"

// Claims mirrors the access-token contract from the architecture docs.
type Claims struct {
	Sub           string         `json:"sub"`
	OrgID         string         `json:"org_id"`
	BranchIDs     []string       `json:"branch_ids"`
	Roles         []string       `json:"roles"`
	Permissions   []string       `json:"permissions"`
	Attrs         map[string]any `json:"attrs"`
	AMR           []string       `json:"amr"`
	SID           string         `json:"sid"`
	OperatorLabel string         `json:"operator_label,omitempty"`
	SessionID     string         `json:"session_id,omitempty"`
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

func (c Claims) ToSubject() authz.Subject {
	op := c.OperatorLabel
	sid := c.SessionID
	if op == "" && c.Attrs != nil {
		if v, ok := c.Attrs["operator_label"].(string); ok {
			op = v
		}
	}
	if sid == "" && c.Attrs != nil {
		if v, ok := c.Attrs["session_id"].(string); ok {
			sid = v
		}
	}
	if sid == "" {
		sid = c.SID
	}
	return authz.Subject{
		Sub:           c.Sub,
		OrgID:         c.OrgID,
		BranchIDs:     c.BranchIDs,
		Roles:         c.Roles,
		Permissions:   c.Permissions,
		Attrs:         c.Attrs,
		AMR:           c.AMR,
		OperatorLabel: op,
		SessionID:     sid,
	}
}

type Validator struct {
	secret     []byte
	issuer     string
	audience   string
	bypass     bool
	jwksURL    string
	httpClient *http.Client

	mu    sync.RWMutex
	keys  map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewValidatorFromEnv() *Validator {
	issuer := envOr("JWT_ISSUER", "http://localhost:8081/realms/nexus")
	jwks := os.Getenv("JWT_JWKS_URL")
	if jwks == "" && os.Getenv("KEYCLOAK_URL") != "" {
		jwks = strings.TrimRight(os.Getenv("KEYCLOAK_URL"), "/") + "/realms/" + envOr("KEYCLOAK_REALM", "nexus") + "/protocol/openid-connect/certs"
	}
	if jwks == "" {
		jwks = strings.TrimRight(issuer, "/") + "/protocol/openid-connect/certs"
	}
	return &Validator{
		secret:   []byte(envOr("DEV_JWT_SECRET", "nexus-dev-secret-change-me")),
		issuer:   issuer,
		audience: envOr("JWT_AUDIENCE", "nexus-api"),
		bypass:   strings.EqualFold(os.Getenv("DEV_AUTH_BYPASS"), "true"),
		jwksURL:  jwks,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		keys: map[string]*rsa.PublicKey{},
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
		claims, err := v.Parse(r.Context(), raw)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid_token","detail":%q}`, err.Error()), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ClaimsContextKey, claims)))
	})
}

func (v *Validator) Parse(ctx context.Context, tokenString string) (Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		switch t.Method.Alg() {
		case jwt.SigningMethodHS256.Alg():
			if !v.bypass && os.Getenv("DEV_JWT_SECRET") == "" {
				return nil, fmt.Errorf("HS256 only allowed in development")
			}
			return v.secret, nil
		case jwt.SigningMethodRS256.Alg():
			kid, _ := t.Header["kid"].(string)
			key, err := v.lookupRSA(ctx, kid)
			if err != nil {
				return nil, err
			}
			return key, nil
		default:
			return nil, fmt.Errorf("unexpected alg %s", t.Method.Alg())
		}
	}, jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience))
	if err != nil {
		return Claims{}, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return Claims{}, errors.New("invalid claims")
	}
	normalizeClaims(claims)
	return *claims, nil
}

func normalizeClaims(c *Claims) {
	if c.OrgID == "" {
		if org, ok := c.Attrs["org_id"].(string); ok {
			c.OrgID = org
		}
	}
	if len(c.Roles) == 0 {
		// Keycloak often puts realm roles under realm_access — accept attrs fallback
		if roles, ok := c.Attrs["roles"].([]any); ok {
			for _, r := range roles {
				if s, ok := r.(string); ok {
					c.Roles = append(c.Roles, s)
				}
			}
		}
	}
	if c.Attrs == nil {
		c.Attrs = map[string]any{}
	}
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (v *Validator) lookupRSA(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	fresh := time.Since(v.fetchedAt) < 10*time.Minute
	v.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}
	if err := v.refreshJWKS(ctx); err != nil {
		if ok {
			return key, nil
		}
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok = v.keys[kid]
	if !ok {
		// try any key if kid missing
		for _, k := range v.keys {
			return k, nil
		}
		return nil, fmt.Errorf("jwks kid not found: %s", kid)
	}
	return key, nil
}

func (v *Validator) refreshJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	res, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("jwks status %d", res.StatusCode)
	}
	var body jwksResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range body.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		kid := k.Kid
		if kid == "" {
			kid = "default"
		}
		keys[kid] = pub
	}
	if len(keys) == 0 {
		return errors.New("jwks contained no RSA keys")
	}
	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func rsaPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nb)
	var eInt int
	for _, b := range eb {
		eInt = eInt<<8 + int(b)
	}
	if eInt == 0 {
		return nil, errors.New("invalid exponent")
	}
	return &rsa.PublicKey{N: n, E: eInt}, nil
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
			"inventory.movement.void.request",
			"inventory.slip.read",
			"inventory.slip.create",
			"inventory.slip.print",
			"approval.read",
			"session.operator",
			"payroll.run.prepare",
			"payroll.run.read",
			"reporting.image.read",
			"reporting.image.create",
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
			"approval.decide",
			"approval.read",
			"session.operator",
			"reporting.image.read",
			"reporting.image.create",
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

// WarehouseManagerClaims — area boss for Almacén Norte; can void in wh_norte only.
func WarehouseManagerClaims() Claims {
	return Claims{
		Sub:       "usr_dev_wh_manager",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte"},
		Roles:     []string{"warehouse_manager"},
		Permissions: []string{
			"inventory.balance.read",
			"inventory.movement.read",
			"inventory.movement.create",
			"inventory.movement.void",
			"inventory.slip.read",
			"inventory.slip.create",
			"inventory.slip.print",
			"approval.decide",
			"approval.read",
			"session.operator",
			"reporting.image.read",
			"reporting.image.create",
		},
		Attrs: map[string]any{
			"max_adjustment":     50000.0,
			"managed_warehouses": []string{"wh_norte"},
		},
		AMR: []string{"pwd", "otp"},
		SID: "sess_dev_wh_manager",
	}
}

// RegionalManagerClaims — superior of area managers in Región Norte.
func RegionalManagerClaims() Claims {
	return Claims{
		Sub:       "usr_dev_regional",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte", "br_sur"},
		Roles:     []string{"regional_manager"},
		Permissions: []string{
			"inventory.balance.read",
			"inventory.movement.read",
			"inventory.movement.create",
			"inventory.movement.void",
			"inventory.slip.read",
			"inventory.slip.create",
			"inventory.slip.print",
			"approval.decide",
			"approval.read",
			"session.operator",
			"payroll.run.read",
			"reporting.image.read",
			"reporting.image.create",
		},
		Attrs: map[string]any{
			"max_adjustment":     50000.0,
			"managed_warehouses": []string{"wh_norte"},
		},
		AMR: []string{"pwd", "otp"},
		SID: "sess_dev_regional",
	}
}

// StoreOwnerClaims — small-shop owner (abarrotes): inventory + labels + photos, no payroll.
func StoreOwnerClaims(sub, orgID, branchCode, storeName string) Claims {
	if sub == "" {
		sub = "usr_dev_owner"
	}
	if orgID == "" {
		orgID = "org_demo"
	}
	if branchCode == "" {
		branchCode = "br_norte"
	}
	return Claims{
		Sub:       sub,
		OrgID:     orgID,
		BranchIDs: []string{branchCode},
		Roles:     []string{"store_owner"},
		Permissions: []string{
			"inventory.balance.read",
			"inventory.movement.create",
			"inventory.movement.read",
			"inventory.movement.void.request",
			"inventory.catalog.read",
			"inventory.label.read",
			"inventory.receipt.read",
			"inventory.receipt.create",
			"inventory.receipt.post",
			"inventory.warehouse.read",
			"inventory.slip.read",
			"inventory.slip.create",
			"inventory.slip.print",
			"approval.read",
			"session.operator",
			"reporting.read",
			"reporting.image.read",
			"reporting.image.create",
			"store.setup.read",
		},
		Attrs: map[string]any{
			"max_adjustment": 100000.0,
			"store_name":     storeName,
			"profile":        "abarrotes",
		},
		AMR: []string{"pwd", "otp"},
		SID: "sess_" + sub,
	}
}

func FromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ClaimsContextKey).(Claims)
	return c, ok
}

// OPAClient is retained for gateway-level checks; services use packages/go/authz.
type OPAClient = authz.Client

func NewOPAClient(baseURL string) *authz.Client {
	if baseURL == "" {
		return authz.NewClientFromEnv()
	}
	return authz.NewClient(baseURL)
}

type OPAInput = authz.Input
