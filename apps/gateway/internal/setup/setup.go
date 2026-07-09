package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

type Status struct {
	NeedsSetup   bool   `json:"needs_setup"`
	Profile      string `json:"profile,omitempty"`
	StoreName    string `json:"store_name,omitempty"`
	CanReinstall bool   `json:"can_reinstall"`
	Message      string `json:"message,omitempty"`
}

type ProductInput struct {
	Name     string  `json:"name"`
	Barcode  string  `json:"barcode,omitempty"`
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Dept     string  `json:"department,omitempty"`
}

type CompleteRequest struct {
	StoreName  string         `json:"store_name"`
	BranchName string         `json:"branch_name"`
	OwnerName  string         `json:"owner_name"`
	OwnerEmail string         `json:"owner_email"`
	Currency   string         `json:"currency"`
	Products   []ProductInput `json:"products"`
	PresetIDs  []string       `json:"preset_ids"`
	Force      bool           `json:"force"`
}

type CompleteResult struct {
	OrgID      string `json:"org_id"`
	OrgCode    string `json:"org_code"`
	BranchID   string `json:"branch_id"`
	BranchCode string `json:"branch_code"`
	OwnerSub   string `json:"owner_sub"`
	StoreName  string `json:"store_name"`
	Profile    string `json:"profile"`
	Products   int    `json:"products_created"`
}

type PresetProduct struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Barcode  string  `json:"barcode"`
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Dept     string  `json:"department"`
	DeptName string  `json:"department_name"`
}

type Service struct {
	Pool *pgxpool.Pool
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	if s.Pool == nil {
		return Status{
			NeedsSetup:   true,
			CanReinstall: true,
			Message:      "Sin base de datos: usa el asistente cuando Postgres esté listo.",
		}, nil
	}
	var completed *time.Time
	var profile, storeName *string
	err := s.Pool.QueryRow(ctx, `
SELECT completed_at, profile, store_name FROM app_install WHERE id = 1`).Scan(&completed, &profile, &storeName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{NeedsSetup: true, CanReinstall: true}, nil
	}
	if err != nil {
		return Status{NeedsSetup: true, CanReinstall: true, Message: err.Error()}, nil
	}
	st := Status{CanReinstall: true}
	if completed != nil {
		st.NeedsSetup = false
		if profile != nil {
			st.Profile = *profile
		}
		if storeName != nil {
			st.StoreName = *storeName
		}
		st.Message = "La tienda ya está configurada."
	} else {
		st.NeedsSetup = true
	}
	return st, nil
}

func Presets() []PresetProduct {
	return []PresetProduct{
		{ID: "leche", Name: "Leche entera 1L", Barcode: "7501000110101", Price: 28.50, Quantity: 24, Dept: "lacteos", DeptName: "Lácteos"},
		{ID: "pan", Name: "Pan de caja", Barcode: "7501000220202", Price: 42.00, Quantity: 18, Dept: "abarrotes", DeptName: "Abarrotes"},
		{ID: "huevos", Name: "Huevo blanco 12 pzas", Barcode: "7501000330303", Price: 48.00, Quantity: 20, Dept: "lacteos", DeptName: "Lácteos"},
		{ID: "agua", Name: "Agua 1.5L", Barcode: "7501000440404", Price: 15.00, Quantity: 36, Dept: "bebidas", DeptName: "Bebidas"},
		{ID: "refresco", Name: "Refresco 600ml", Barcode: "7501000550505", Price: 18.00, Quantity: 48, Dept: "bebidas", DeptName: "Bebidas"},
		{ID: "jabon", Name: "Jabón de barra", Barcode: "7501000660606", Price: 22.00, Quantity: 30, Dept: "limpieza", DeptName: "Limpieza"},
		{ID: "arroz", Name: "Arroz 1kg", Barcode: "7501000770707", Price: 35.00, Quantity: 40, Dept: "abarrotes", DeptName: "Abarrotes"},
		{ID: "frijol", Name: "Frijol 1kg", Barcode: "7501000880808", Price: 38.00, Quantity: 30, Dept: "abarrotes", DeptName: "Abarrotes"},
		{ID: "galleta", Name: "Galletas surtidas", Barcode: "7501000990909", Price: 20.00, Quantity: 25, Dept: "dulces", DeptName: "Dulces"},
		{ID: "papel", Name: "Papel higiénico 4R", Barcode: "7501001010101", Price: 45.00, Quantity: 16, Dept: "limpieza", DeptName: "Limpieza"},
	}
}

func (s *Service) Complete(ctx context.Context, req CompleteRequest) (CompleteResult, error) {
	if s.Pool == nil {
		return CompleteResult{}, errors.New("database_required")
	}
	req.StoreName = strings.TrimSpace(req.StoreName)
	req.BranchName = strings.TrimSpace(req.BranchName)
	req.OwnerName = strings.TrimSpace(req.OwnerName)
	req.OwnerEmail = strings.ToLower(strings.TrimSpace(req.OwnerEmail))
	if req.Currency == "" {
		req.Currency = "MXN"
	}
	if req.StoreName == "" || req.BranchName == "" || req.OwnerName == "" || req.OwnerEmail == "" {
		return CompleteResult{}, errors.New("store_branch_owner_required")
	}
	if !strings.Contains(req.OwnerEmail, "@") {
		return CompleteResult{}, errors.New("invalid_email")
	}

	st, err := s.Status(ctx)
	if err != nil {
		return CompleteResult{}, err
	}
	if !st.NeedsSetup && !req.Force {
		return CompleteResult{}, errors.New("already_configured")
	}

	products := mergeProducts(req)
	if len(products) == 0 {
		return CompleteResult{}, errors.New("products_required")
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return CompleteResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := db.SetRLSBypass(ctx, tx, true); err != nil {
		return CompleteResult{}, err
	}

	orgID := uuid.New()
	orgCode := uniqueCode(req.StoreName)
	branchID := uuid.New()
	branchCode := "tienda"
	whID := uuid.New()
	ownerID := uuid.New()
	ownerSub := "usr_owner_" + shortID()
	roleID := uuid.New()

	if _, err = tx.Exec(ctx, `
INSERT INTO organizations (id, code, name, status, setup_completed_at, profile)
VALUES ($1, $2, $3, 'ACTIVE', now(), 'abarrotes')`, orgID, orgCode, req.StoreName); err != nil {
		return CompleteResult{}, fmt.Errorf("org: %w", err)
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO branches (id, org_id, code, name, region)
VALUES ($1, $2, $3, $4, 'LOCAL')`, branchID, orgID, branchCode, req.BranchName); err != nil {
		return CompleteResult{}, fmt.Errorf("branch: %w", err)
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO warehouses (id, branch_id, code, name)
VALUES ($1, $2, 'principal', 'Almacén de llegada')`, whID, branchID); err != nil {
		return CompleteResult{}, fmt.Errorf("warehouse: %w", err)
	}
	// Prefer ARRIVAL kind when migration 015 is applied (small-shop receiving bay).
	_, _ = tx.Exec(ctx, `UPDATE warehouses SET warehouse_kind = 'ARRIVAL' WHERE id = $1`, whID)
	if err := ensurePermissions(ctx, tx); err != nil {
		return CompleteResult{}, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO roles (id, org_id, code, name) VALUES ($1, $2, 'store_owner', 'Dueño de tienda')`, roleID, orgID); err != nil {
		return CompleteResult{}, fmt.Errorf("role: %w", err)
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO role_permissions (role_id, permission_id)
SELECT $1, p.id FROM permissions p
WHERE p.code IN (
  'inventory.balance.read','inventory.movement.create','inventory.movement.read',
  'inventory.catalog.read','inventory.label.read','reporting.read',
  'reporting.image.read','reporting.image.create','store.setup.read',
  'inventory.receipt.read','inventory.receipt.create','inventory.receipt.post',
  'inventory.warehouse.read','mail.read','mail.send','mail.announce',
  'store.storefront.read','store.storefront.manage',
  'store.department.manager.read','store.department.manager.assign'
)`, roleID); err != nil {
		return CompleteResult{}, fmt.Errorf("role_perms: %w", err)
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO users (id, org_id, idp_sub, email, display_name, mfa_methods)
VALUES ($1, $2, $3, $4, $5, ARRAY['pwd','otp'])`, ownerID, orgID, ownerSub, req.OwnerEmail, req.OwnerName); err != nil {
		return CompleteResult{}, fmt.Errorf("user: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id, branch_id) VALUES ($1, $2, $3)`, ownerID, roleID, branchID); err != nil {
		return CompleteResult{}, fmt.Errorf("user_role: %w", err)
	}
	_, _ = tx.Exec(ctx, `INSERT INTO user_attributes (user_id, attr_key, attr_value) VALUES ($1, 'max_adjustment', '100000')`, ownerID)

	depts := []struct{ code, name string }{
		{"abarrotes", "Abarrotes"},
		{"bebidas", "Bebidas"},
		{"lacteos", "Lácteos"},
		{"dulces", "Dulces"},
		{"limpieza", "Limpieza"},
	}
	deptIDs := map[string]uuid.UUID{}
	for i, d := range depts {
		id := uuid.New()
		deptIDs[d.code] = id
		if _, err = tx.Exec(ctx, `
INSERT INTO store_departments (id, org_id, branch_id, code, name, sort_order)
VALUES ($1, $2, $3, $4, $5, $6)`, id, orgID, branchID, d.code, d.name, (i+1)*10); err != nil {
			return CompleteResult{}, fmt.Errorf("department %s: %w", d.code, err)
		}
	}
	deptNames := map[string]string{}
	for _, d := range depts {
		deptNames[d.code] = d.name
	}

	created := 0
	for i, p := range products {
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			continue
		}
		if p.Quantity < 0 {
			p.Quantity = 0
		}
		dept := p.Dept
		if dept == "" {
			dept = "abarrotes"
		}
		skuBase := fmt.Sprintf("SKU%03d", i+1)
		prodID := uuid.New()
		skuID := uuid.New()
		skuCode := orgCode + "-" + skuBase
		if _, err = tx.Exec(ctx, `
INSERT INTO products (id, org_id, sku_base, name) VALUES ($1, $2, $3, $4)`, prodID, orgID, skuBase, p.Name); err != nil {
			return CompleteResult{}, fmt.Errorf("product: %w", err)
		}
		if _, err = tx.Exec(ctx, `
INSERT INTO product_skus (id, product_id, sku, uom, barcode, brand)
VALUES ($1, $2, $3, 'EA', NULLIF($4,''), $5)`, skuID, prodID, skuCode, p.Barcode, req.StoreName); err != nil {
			return CompleteResult{}, fmt.Errorf("sku: %w", err)
		}
		if _, err = tx.Exec(ctx, `
INSERT INTO stock_balances (warehouse_id, sku_id, on_hand, version)
VALUES ($1, $2, $3, 1)`, whID, skuID, p.Quantity); err != nil {
			return CompleteResult{}, fmt.Errorf("stock: %w", err)
		}
		if deptID, ok := deptIDs[dept]; ok {
			_, _ = tx.Exec(ctx, `
INSERT INTO product_placements (product_id, department_id, is_primary, active)
VALUES ($1, $2, TRUE, TRUE)`, prodID, deptID)
		}
		if p.Price > 0 {
			_, _ = tx.Exec(ctx, `
INSERT INTO store_sku_labels (
  org_id, branch_id, sku_id, store_display_name, public_description,
  department_label, currency, common_price, price_mode, active
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'COMMON', TRUE)`,
				orgID, branchID, skuID, req.StoreName, p.Name, deptNames[dept], req.Currency, p.Price)
		}
		created++
	}

	if _, err = tx.Exec(ctx, `
INSERT INTO app_install (id, completed_at, profile, store_name, owner_sub, org_id, branch_code, meta, updated_at)
VALUES (1, now(), 'abarrotes', $1, $2, $3, $4, $5::jsonb, now())
ON CONFLICT (id) DO UPDATE SET
  completed_at = excluded.completed_at,
  profile = excluded.profile,
  store_name = excluded.store_name,
  owner_sub = excluded.owner_sub,
  org_id = excluded.org_id,
  branch_code = excluded.branch_code,
  meta = excluded.meta,
  updated_at = now()`,
		req.StoreName, ownerSub, orgID, branchCode,
		mustJSON(map[string]any{"currency": req.Currency, "products": created})); err != nil {
		return CompleteResult{}, fmt.Errorf("app_install: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CompleteResult{}, err
	}

	return CompleteResult{
		OrgID:      orgID.String(),
		OrgCode:    orgCode,
		BranchID:   branchID.String(),
		BranchCode: branchCode,
		OwnerSub:   ownerSub,
		StoreName:  req.StoreName,
		Profile:    "abarrotes",
		Products:   created,
	}, nil
}

func ensurePermissions(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.movement.create', 'inventory', 'create', 'movement'),
  ('inventory.balance.read', 'inventory', 'read', 'balance'),
  ('inventory.movement.read', 'inventory', 'read', 'movement'),
  ('inventory.catalog.read', 'inventory', 'read', 'catalog'),
  ('inventory.label.read', 'inventory', 'read', 'label'),
  ('reporting.read', 'reporting', 'read', 'report'),
  ('reporting.image.read', 'reporting', 'read', 'image'),
  ('reporting.image.create', 'reporting', 'create', 'image'),
  ('store.setup.read', 'store', 'read', 'setup'),
  ('inventory.receipt.read', 'inventory', 'read', 'receipt'),
  ('inventory.receipt.create', 'inventory', 'create', 'receipt'),
  ('inventory.receipt.post', 'inventory', 'post', 'receipt'),
  ('inventory.warehouse.read', 'inventory', 'read', 'warehouse'),
  ('mail.read', 'mail', 'read', 'message'),
  ('mail.send', 'mail', 'send', 'message'),
  ('mail.announce', 'mail', 'announce', 'announcement'),
  ('store.storefront.read', 'store', 'read', 'storefront'),
  ('store.storefront.manage', 'store', 'manage', 'storefront'),
  ('store.department.manager.read', 'store', 'read', 'department_manager'),
  ('store.department.manager.assign', 'store', 'assign', 'department_manager')
ON CONFLICT (code) DO NOTHING`)
	return err
}

func mergeProducts(req CompleteRequest) []ProductInput {
	byID := map[string]PresetProduct{}
	for _, p := range Presets() {
		byID[p.ID] = p
	}
	out := make([]ProductInput, 0, len(req.Products)+len(req.PresetIDs))
	seen := map[string]bool{}
	for _, id := range req.PresetIDs {
		if p, ok := byID[id]; ok && !seen[p.Name] {
			seen[p.Name] = true
			out = append(out, ProductInput{
				Name: p.Name, Barcode: p.Barcode, Price: p.Price, Quantity: p.Quantity, Dept: p.Dept,
			})
		}
	}
	for _, p := range req.Products {
		name := strings.TrimSpace(p.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, p)
	}
	return out
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func uniqueCode(name string) string {
	s := strings.ToLower(stripAccents(name))
	s = nonAlnum.ReplaceAllString(s, "")
	if len(s) > 12 {
		s = s[:12]
	}
	if s == "" {
		s = "tienda"
	}
	return s + shortID()
}

func stripAccents(s string) string {
	repl := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
		"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ü", "u", "Ñ", "n",
	)
	return repl.Replace(s)
}

func shortID() string {
	return strings.ReplaceAll(uuid.NewString()[:8], "-", "")
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
