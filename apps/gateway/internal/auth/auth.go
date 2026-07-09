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
			"inventory.transport.read",
			"pos.sale.read",
			"pos.sale.create",
			"pos.invoice.request",
			"pos.invoice.read",
			"inventory.transport.create",
			"inventory.transport.print",
			"approval.read",
			"session.operator",
			"payroll.run.prepare",
			"payroll.run.read",
			"employee.read",
			"inventory.adjustment.create",
			"inventory.adjustment.read",
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
			"employee.read",
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
			"employee.read",
			"employee.write",
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
			"inventory.adjustment.create",
			"inventory.adjustment.read",
			"inventory.slip.read",
			"inventory.slip.create",
			"inventory.slip.print",
			"inventory.transport.read",
			"inventory.transport.create",
			"inventory.transport.print",
			"approval.decide",
			"approval.read",
			"session.operator",
			"reporting.image.read",
			"reporting.image.create",
			"store.department.manager.read",
			"employee.read",
			"pos.sale.read",
			"pos.sale.create",
			"pos.invoice.request",
			"pos.invoice.read",
		},
		Attrs: map[string]any{
			"max_adjustment":      50000.0,
			"managed_warehouses":  []string{"wh_norte"},
			"managed_departments": []string{"electronica", "jugueteria"},
		},
		AMR: []string{"pwd", "otp"},
		SID: "sess_dev_wh_manager",
	}
}

// SecurityOfficerClaims — gate / vigilance: logistics reports + seal verify (no stock writes).
func SecurityOfficerClaims() Claims {
	return Claims{
		Sub:       "usr_dev_security",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte", "br_sur", "br_cedi"},
		Roles:     []string{"security_officer"},
		Permissions: []string{
			"reporting.security.read",
			"inventory.seal.verify",
			"inventory.slip.read",
			"inventory.transport.read",
			"inventory.parcel.read",
			"inventory.transfer.read",
			"inventory.shipment.read",
			"inventory.balance.read",
			"reporting.image.read",
			"reporting.image.create",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_security",
	}
}

func WarehouseClerkClaims() Claims {
	return Claims{
		Sub:       "usr_dev_warehouse",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte"},
		Roles:     []string{"warehouse_clerk"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read", "inventory.label.read",
			"inventory.warehouse.read", "inventory.movement.create", "inventory.movement.read",
			"inventory.movement.void.request", "inventory.receipt.read", "inventory.receipt.create",
			"inventory.receipt.post", "inventory.transfer.read", "inventory.transfer.create",
			"inventory.transfer.ship", "inventory.transfer.receive",
			"inventory.slip.read", "inventory.slip.create", "inventory.slip.print",
			"inventory.slip.ship", "inventory.slip.receive",
			"inventory.adjustment.create", "inventory.adjustment.read",
			"session.operator", "approval.read", "reporting.image.read", "reporting.image.create",
		},
		Attrs: map[string]any{"max_adjustment": 10000.0},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_warehouse",
	}
}

func CEDIClerkClaims() Claims {
	return Claims{
		Sub:       "usr_dev_cedi",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_cedi"},
		Roles:     []string{"cedi_clerk"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read", "inventory.label.read",
			"inventory.warehouse.read", "inventory.movement.create", "inventory.movement.read",
			"inventory.receipt.read", "inventory.receipt.create", "inventory.receipt.post",
			"inventory.shipment.read", "inventory.shipment.create", "inventory.shipment.post",
			"inventory.transfer.read", "inventory.transfer.create", "inventory.transfer.ship",
			"inventory.transfer.receive", "inventory.parcel.read",
			"inventory.slip.read", "inventory.slip.create", "inventory.slip.print",
			"inventory.slip.ship", "inventory.slip.receive",
			"inventory.transport.read", "inventory.transport.create", "inventory.transport.print",
			"inventory.transport.depart", "inventory.transport.deliver",
			"inventory.adjustment.create", "inventory.adjustment.read",
			"inventory.seal.verify", "reporting.security.read",
			"session.operator", "approval.read", "reporting.image.read", "reporting.image.create",
		},
		Attrs: map[string]any{"max_adjustment": 25000.0},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_cedi",
	}
}

func DispatchClerkClaims() Claims {
	return Claims{
		Sub:       "usr_dev_dispatch",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_cedi", "br_norte"},
		Roles:     []string{"dispatch_clerk"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read",
			"inventory.parcel.read", "inventory.slip.read", "inventory.slip.create",
			"inventory.slip.print", "inventory.slip.ship", "inventory.slip.receive",
			"inventory.transport.read", "inventory.transport.create", "inventory.transport.print",
			"inventory.transport.depart", "inventory.transport.deliver",
			"inventory.transfer.read", "inventory.transfer.ship", "inventory.transfer.receive",
			"inventory.seal.verify", "reporting.security.read",
			"session.operator", "reporting.image.read", "reporting.image.create",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_dispatch",
	}
}

func SalesAssociateClaims() Claims {
	return Claims{
		Sub:       "usr_dev_sales",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte"},
		Roles:     []string{"sales_associate"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read", "inventory.label.read",
			"customer.read", "customer.card.read",
			"pos.sale.read", "pos.sale.create", "pos.invoice.request", "pos.invoice.read",
			"session.operator", "reporting.image.read", "reporting.image.create",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_sales",
	}
}

func CashierClaims() Claims {
	return Claims{
		Sub:       "usr_dev_cashier",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte"},
		Roles:     []string{"cashier"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read", "inventory.label.read",
			"customer.read", "customer.card.read",
			"pos.sale.read", "pos.sale.create", "pos.sale.void",
			"pos.invoice.request", "pos.invoice.read",
			"session.operator", "approval.read",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_cashier",
	}
}

func WarrantyClerkClaims() Claims {
	return Claims{
		Sub:       "usr_dev_warranty",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte"},
		Roles:     []string{"warranty_clerk"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read",
			"inventory.warranty.read", "inventory.warranty.create", "inventory.warranty.manage",
			"inventory.return.read", "inventory.return.create",
			"inventory.parcel.read", "inventory.slip.read", "inventory.slip.create", "inventory.slip.print",
			"inventory.transfer.read", "inventory.transfer.create",
			"customer.read", "session.operator",
			"reporting.image.read", "reporting.image.create",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_warranty",
	}
}

func EcommerceClerkClaims() Claims {
	return Claims{
		Sub:       "usr_dev_ecommerce",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte"},
		Roles:     []string{"ecommerce_clerk"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read", "inventory.label.read",
			"store.storefront.read", "store.storefront.manage",
			"customer.read", "customer.manage", "customer.card.read", "customer.card.manage",
			"pos.sale.read", "pos.invoice.read",
			"inventory.parcel.read", "inventory.slip.read", "inventory.slip.create", "inventory.slip.print",
			"session.operator", "reporting.image.read", "reporting.image.create",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_ecommerce",
	}
}

func StoreCoordinatorClaims() Claims {
	return Claims{
		Sub:       "usr_dev_coordinator",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte"},
		Roles:     []string{"store_coordinator"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read", "inventory.label.read", "inventory.label.manage",
			"inventory.warehouse.read", "inventory.movement.create", "inventory.movement.read",
			"inventory.movement.void.request", "inventory.receipt.read", "inventory.receipt.create", "inventory.receipt.post",
			"inventory.transfer.read", "inventory.transfer.create", "inventory.transfer.ship", "inventory.transfer.receive",
			"inventory.slip.read", "inventory.slip.create", "inventory.slip.print", "inventory.slip.ship", "inventory.slip.receive",
			"inventory.adjustment.create", "inventory.adjustment.read",
			"inventory.parcel.read", "inventory.warranty.read", "inventory.return.read",
			"customer.read", "customer.manage", "customer.card.read", "customer.card.manage",
			"pos.sale.read", "pos.sale.create", "pos.sale.void", "pos.invoice.request", "pos.invoice.read",
			"store.department.manager.read", "approval.read", "approval.decide",
			"session.operator", "reporting.read", "reporting.image.read", "reporting.image.create",
			"facilities.workorder.read", "facilities.workorder.create",
		},
		Attrs: map[string]any{"max_adjustment": 25000.0},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_coordinator",
	}
}

func StoreAdminClaims() Claims {
	return Claims{
		Sub:       "usr_dev_store_admin",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte"},
		Roles:     []string{"store_admin"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read", "inventory.label.read", "inventory.label.manage",
			"inventory.warehouse.read", "inventory.movement.create", "inventory.movement.read", "inventory.movement.void",
			"inventory.movement.void.request", "inventory.receipt.read", "inventory.receipt.create", "inventory.receipt.post",
			"inventory.transfer.read", "inventory.transfer.create", "inventory.transfer.ship", "inventory.transfer.receive",
			"inventory.transfer.cancel", "inventory.slip.read", "inventory.slip.create", "inventory.slip.print",
			"inventory.slip.ship", "inventory.slip.receive", "inventory.slip.cancel",
			"inventory.transport.read", "inventory.transport.create", "inventory.transport.print",
			"inventory.adjustment.create", "inventory.adjustment.read",
			"inventory.parcel.read", "inventory.warranty.read", "inventory.warranty.manage",
			"inventory.return.read", "inventory.return.manage",
			"customer.read", "customer.manage", "customer.card.read", "customer.card.manage",
			"pos.sale.read", "pos.sale.create", "pos.sale.void", "pos.invoice.request", "pos.invoice.read", "pos.settings.manage",
			"store.storefront.read", "store.storefront.manage",
			"store.department.manager.read", "store.department.manager.assign",
			"employee.read", "approval.read", "approval.decide",
			"session.operator", "reporting.read", "reporting.image.read", "reporting.image.create",
			"reporting.security.read", "inventory.seal.verify",
			"facilities.workorder.read", "facilities.workorder.create", "facilities.workorder.close",
			"purchasing.order.read",
		},
		Attrs: map[string]any{"max_adjustment": 50000.0, "managed_warehouses": []string{"wh_norte"}},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_store_admin",
	}
}

func AreaManagerClaims() Claims {
	return Claims{
		Sub:       "usr_dev_area",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte"},
		Roles:     []string{"area_manager"},
		Permissions: []string{
			"inventory.balance.read", "inventory.catalog.read", "inventory.label.read",
			"inventory.warehouse.read", "inventory.movement.create", "inventory.movement.read", "inventory.movement.void",
			"inventory.receipt.read", "inventory.receipt.create", "inventory.receipt.post",
			"inventory.transfer.read", "inventory.transfer.create", "inventory.transfer.ship",
			"inventory.transfer.receive", "inventory.transfer.cancel",
			"inventory.slip.read", "inventory.slip.create", "inventory.slip.print", "inventory.slip.cancel",
			"inventory.transport.read", "inventory.transport.create", "inventory.transport.print", "inventory.transport.cancel",
			"inventory.adjustment.create", "inventory.adjustment.read",
			"inventory.parcel.read", "inventory.shipment.read",
			"approval.read", "approval.decide", "session.operator",
			"reporting.read", "reporting.image.read", "reporting.image.create",
			"reporting.security.read", "inventory.seal.verify",
			"store.department.manager.read", "pos.sale.read", "pos.sale.create",
		},
		Attrs: map[string]any{
			"max_adjustment":     50000.0,
			"managed_warehouses": []string{"wh_norte"},
		},
		AMR: []string{"pwd", "otp"},
		SID: "sess_dev_area",
	}
}

func PurchasingClerkClaims() Claims {
	return Claims{
		Sub:       "usr_dev_purchasing",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_cedi", "br_norte"},
		Roles:     []string{"purchasing_clerk"},
		Permissions: []string{
			"purchasing.order.read", "purchasing.order.create",
			"inventory.balance.read", "inventory.catalog.read", "inventory.warehouse.read",
			"inventory.receipt.read", "inventory.shipment.read",
			"session.operator", "reporting.read", "approval.read",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_purchasing",
	}
}

func PurchasingManagerClaims() Claims {
	return Claims{
		Sub:       "usr_dev_purchasing_mgr",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_cedi", "br_norte"},
		Roles:     []string{"purchasing_manager"},
		Permissions: []string{
			"purchasing.order.read", "purchasing.order.create", "purchasing.order.approve",
			"inventory.balance.read", "inventory.catalog.read", "inventory.warehouse.read",
			"inventory.receipt.read", "inventory.receipt.create", "inventory.shipment.read",
			"inventory.shipment.create", "approval.read", "approval.decide",
			"session.operator", "reporting.read",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_purchasing_mgr",
	}
}

func FacilitiesStaffClaims() Claims {
	return Claims{
		Sub:       "usr_dev_facilities",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte", "br_cedi"},
		Roles:     []string{"facilities_staff"},
		Permissions: []string{
			"facilities.workorder.read", "facilities.workorder.create", "facilities.workorder.close",
			"reporting.image.read", "reporting.image.create",
			"session.operator", "inventory.balance.read",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_facilities",
	}
}

func HROfficerClaims() Claims {
	return Claims{
		Sub:       "usr_dev_hr",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte", "br_sur"},
		Roles:     []string{"hr_officer"},
		Permissions: []string{
			"employee.read", "employee.write", "payroll.run.read",
		},
		Attrs: map[string]any{},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_hr",
	}
}

// WebmasterClaims — broad read + print/export of administered domains.
func WebmasterClaims() Claims {
	return Claims{
		Sub:       "usr_dev_webmaster",
		OrgID:     "org_demo",
		BranchIDs: []string{"br_norte", "br_sur", "br_cedi"},
		Roles:     []string{"webmaster"},
		Permissions: []string{
			"reporting.read", "reporting.export", "reporting.print", "reporting.catalog.read",
			"reporting.image.read", "reporting.security.read",
			"inventory.balance.read", "inventory.catalog.read", "inventory.label.read",
			"inventory.warehouse.read", "inventory.movement.read",
			"inventory.receipt.read", "inventory.shipment.read",
			"inventory.transfer.read", "inventory.slip.read", "inventory.transport.read",
			"inventory.parcel.read", "inventory.warranty.read", "inventory.return.read",
			"inventory.adjustment.read", "inventory.seal.verify",
			"pos.sale.read", "pos.invoice.read", "customer.read", "customer.card.read",
			"employee.read", "payroll.run.read",
			"store.storefront.read", "store.department.manager.read",
			"purchasing.order.read", "facilities.workorder.read",
			"approval.read", "session.operator",
		},
		Attrs: map[string]any{"profile": "webmaster"},
		AMR:   []string{"pwd", "otp"},
		SID:   "sess_dev_webmaster",
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
			"inventory.transport.read",
			"inventory.transport.create",
			"inventory.transport.print",
			"reporting.security.read",
			"inventory.seal.verify",
			"approval.decide",
			"approval.read",
			"session.operator",
			"payroll.run.read",
			"reporting.image.read",
			"reporting.image.create",
			"store.department.manager.read",
			"store.department.manager.assign",
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
			"inventory.transport.read",
			"inventory.transport.create",
			"inventory.transport.print",
			"customer.read",
			"customer.manage",
			"customer.card.read",
			"customer.card.manage",
			"approval.read",
			"session.operator",
			"reporting.read",
			"reporting.image.read",
			"reporting.image.create",
			"store.setup.read",
			"store.storefront.read",
			"store.storefront.manage",
			"store.department.manager.read",
			"store.department.manager.assign",
			"pos.sale.read",
			"pos.sale.create",
			"pos.sale.void",
			"pos.invoice.request",
			"pos.invoice.read",
			"pos.settings.manage",
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

// CustomerClaims — end-customer account for storefront self-service (cards).
func CustomerClaims(customerID, orgID, email, displayName, branchCode string) Claims {
	if branchCode == "" {
		branchCode = "*"
	}
	return Claims{
		Sub:       "cust_" + customerID,
		OrgID:     orgID,
		BranchIDs: []string{branchCode},
		Roles:     []string{"customer"},
		Permissions: []string{
			"customer.self.read",
			"customer.card.self",
		},
		Attrs: map[string]any{
			"profile":       "customer",
			"customer_id":   customerID,
			"email":         email,
			"display_name":  displayName,
		},
		AMR: []string{"pwd"},
		SID: "sess_cust_" + customerID,
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
