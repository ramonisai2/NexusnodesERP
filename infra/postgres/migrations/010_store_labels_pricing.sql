-- 010_store_labels_pricing.sql
-- Label printer data: per-store public copy + common/special/final prices,
-- plus SKU attributes (material, barcode, size, color, brand).

SELECT set_config('app.rls_bypass', 'on', false);

ALTER TABLE product_skus
  ADD COLUMN IF NOT EXISTS material_code TEXT,
  ADD COLUMN IF NOT EXISTS barcode TEXT,
  ADD COLUMN IF NOT EXISTS size_code TEXT,
  ADD COLUMN IF NOT EXISTS color_code TEXT,
  ADD COLUMN IF NOT EXISTS brand TEXT,
  ADD COLUMN IF NOT EXISTS extra_attrs JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE UNIQUE INDEX IF NOT EXISTS product_skus_material_code_uidx
  ON product_skus (material_code)
  WHERE material_code IS NOT NULL AND material_code <> '';

CREATE UNIQUE INDEX IF NOT EXISTS product_skus_barcode_uidx
  ON product_skus (barcode)
  WHERE barcode IS NOT NULL AND barcode <> '';

-- Per-store label content and pricing for the label printer.
-- price_mode: COMMON | SPECIAL | FINAL (FINAL = clearance until stock out).
CREATE TABLE IF NOT EXISTS store_sku_labels (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  sku_id UUID NOT NULL REFERENCES product_skus(id) ON DELETE CASCADE,
  store_display_name TEXT, -- e.g. "LA MARINA" printed on hangtag
  public_description TEXT NOT NULL,
  department_label TEXT,   -- e.g. "ROPA DEPORTIVA" as printed on label
  brand_label TEXT,        -- optional override of sku.brand for this store
  extra_descriptions JSONB NOT NULL DEFAULT '{}'::jsonb,
  currency TEXT NOT NULL DEFAULT 'MXN',
  common_price NUMERIC(18,2),
  special_price NUMERIC(18,2),
  final_price NUMERIC(18,2),
  price_mode TEXT NOT NULL DEFAULT 'COMMON'
    CHECK (price_mode IN ('COMMON', 'SPECIAL', 'FINAL')),
  active BOOLEAN NOT NULL DEFAULT TRUE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (branch_id, sku_id)
);

CREATE INDEX IF NOT EXISTS store_sku_labels_branch_idx
  ON store_sku_labels (branch_id)
  WHERE active = TRUE;

CREATE INDEX IF NOT EXISTS store_sku_labels_sku_idx
  ON store_sku_labels (sku_id);

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.label.read', 'inventory', 'read', 'label'),
  ('inventory.label.manage', 'inventory', 'manage', 'label')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('inventory.label.read', 'inventory.label.manage')
WHERE r.code IN ('warehouse_manager', 'regional_manager', 'platform_admin')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'inventory.label.read'
WHERE r.code IN ('inventory_clerk', 'payroll_approver')
ON CONFLICT DO NOTHING;

-- Demo retail SKU inspired by hangtag: Club América jersey / LA MARINA style
INSERT INTO products (id, org_id, sku_base, name) VALUES
  ('66666666-6666-6666-6666-666666666620', '11111111-1111-1111-1111-111111111111',
   'CAAL686', 'Camiseta manga corta de hombre Club América')
ON CONFLICT (org_id, sku_base) DO NOTHING;

INSERT INTO product_skus (id, product_id, sku, uom, material_code, barcode, size_code, color_code, brand, extra_attrs)
VALUES (
  '77777777-7777-7777-7777-777777777720',
  '66666666-6666-6666-6666-666666666620',
  'CAAL686101YL1',
  'EA',
  'CAAL686101YL1',
  '7450130556398',
  'M',
  'YL1',
  'FEXPRO',
  '{"license":"Club América","gender":"hombre","sleeve":"corta"}'::jsonb
)
ON CONFLICT (sku) DO UPDATE SET
  material_code = EXCLUDED.material_code,
  barcode = EXCLUDED.barcode,
  size_code = EXCLUDED.size_code,
  color_code = EXCLUDED.color_code,
  brand = EXCLUDED.brand,
  extra_attrs = EXCLUDED.extra_attrs;

-- Place under a sportswear-like department if present; otherwise create Ropa Deportiva at Norte
INSERT INTO store_departments (id, org_id, branch_id, code, name, sort_order) VALUES
  ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb03', '11111111-1111-1111-1111-111111111111',
   '22222222-2222-2222-2222-222222222201', 'ropa_deportiva', 'Ropa Deportiva', 30)
ON CONFLICT (branch_id, code) DO NOTHING;

INSERT INTO department_categories (id, department_id, code, name, sort_order) VALUES
  ('cccccccc-cccc-cccc-cccc-cccccccccc21', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb03',
   'playeras', 'Playeras', 10)
ON CONFLICT (department_id, code) DO NOTHING;

INSERT INTO product_placements (product_id, department_id, category_id, is_primary) VALUES
  ('66666666-6666-6666-6666-666666666620', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb03',
   'cccccccc-cccc-cccc-cccc-cccccccccc21', TRUE)
ON CONFLICT (product_id, department_id, category_id) DO NOTHING;

INSERT INTO stock_balances (warehouse_id, sku_id, on_hand, version) VALUES
  ('55555555-5555-5555-5555-555555555501', '77777777-7777-7777-7777-777777777720', 48, 1),
  ('55555555-5555-5555-5555-555555555502', '77777777-7777-7777-7777-777777777720', 20, 1)
ON CONFLICT (warehouse_id, sku_id) DO NOTHING;

-- Also enrich existing retail SKUs with material/barcode for label demos
UPDATE product_skus SET
  material_code = COALESCE(NULLIF(material_code, ''), sku),
  barcode = COALESCE(NULLIF(barcode, ''), CASE sku
    WHEN 'LAPTOP-14' THEN '7501000000014'
    WHEN 'PHONE-X' THEN '7501000000015'
    WHEN 'FIG-COL-01' THEN '7501000000016'
    ELSE '7501000000099'
  END),
  size_code = COALESCE(size_code, 'U'),
  color_code = COALESCE(color_code, 'STD'),
  brand = COALESCE(brand, 'NEXUS')
WHERE sku IN ('LAPTOP-14', 'PHONE-X', 'FIG-COL-01');

INSERT INTO store_sku_labels (
  org_id, branch_id, sku_id, store_display_name, public_description, department_label,
  brand_label, extra_descriptions, currency, common_price, special_price, final_price, price_mode
) VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222201',
    '77777777-7777-7777-7777-777777777720',
    'LA MARINA',
    'CAMISETA MANGA CORTA DE HOMBRE',
    'ROPA DEPORTIVA',
    'FEXPRO',
    '{"license":"Producto oficial Club América","fit":"Regular"}'::jsonb,
    'MXN', 899.00, 749.00, 499.00, 'COMMON'
  ),
  (
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222202',
    '77777777-7777-7777-7777-777777777720',
    'LA MARINA SUR',
    'CAMISETA MANGA CORTA DE HOMBRE',
    'ROPA DEPORTIVA',
    'FEXPRO',
    '{"license":"Producto oficial Club América"}'::jsonb,
    'MXN', 879.00, 699.00, 449.00, 'SPECIAL'
  ),
  (
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222201',
    '77777777-7777-7777-7777-777777777712',
    'NEXUS NORTE',
    'FIGURA COLECCIONABLE EDICIÓN LIMITADA',
    'JUGUETERÍA',
    'NEXUS',
    '{"series":"Edición limitada"}'::jsonb,
    'MXN', 599.00, 499.00, 299.00, 'FINAL'
  ),
  (
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222201',
    '77777777-7777-7777-7777-777777777710',
    'NEXUS NORTE',
    'LAPTOP 14 PULGADAS',
    'ELECTRÓNICA',
    'NEXUS',
    '{}'::jsonb,
    'MXN', 12999.00, 11999.00, NULL, 'COMMON'
  )
ON CONFLICT (branch_id, sku_id) DO UPDATE SET
  store_display_name = EXCLUDED.store_display_name,
  public_description = EXCLUDED.public_description,
  department_label = EXCLUDED.department_label,
  brand_label = EXCLUDED.brand_label,
  extra_descriptions = EXCLUDED.extra_descriptions,
  common_price = EXCLUDED.common_price,
  special_price = EXCLUDED.special_price,
  final_price = EXCLUDED.final_price,
  price_mode = EXCLUDED.price_mode,
  updated_at = now();

ALTER TABLE store_sku_labels ENABLE ROW LEVEL SECURITY;
ALTER TABLE store_sku_labels FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON store_sku_labels;
CREATE POLICY org_isolation ON store_sku_labels
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));
