-- 009_departments.sql
-- Store departments + nested categories; products may appear in multiple placements.

SELECT set_config('app.rls_bypass', 'on', false);

-- Top-level departments of a store/branch (Electrónica, Juguetería, …)
CREATE TABLE IF NOT EXISTS store_departments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  UNIQUE (branch_id, code)
);

CREATE INDEX IF NOT EXISTS store_departments_branch_idx
  ON store_departments (branch_id, sort_order, code);

-- Categories inside a department (Cómputo, Video, Telefonía, Videojuegos, …)
CREATE TABLE IF NOT EXISTS department_categories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  department_id UUID NOT NULL REFERENCES store_departments(id) ON DELETE CASCADE,
  parent_id UUID REFERENCES department_categories(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  UNIQUE (department_id, code)
);

CREATE INDEX IF NOT EXISTS department_categories_dept_idx
  ON department_categories (department_id, sort_order, code);

-- A product (article) can be placed in several department/category slots
-- e.g. collectible in Electrónica/Videojuegos AND Juguetería/Coleccionables.
CREATE TABLE IF NOT EXISTS product_placements (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  department_id UUID NOT NULL REFERENCES store_departments(id) ON DELETE CASCADE,
  category_id UUID REFERENCES department_categories(id) ON DELETE SET NULL,
  is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  UNIQUE (product_id, department_id, category_id)
);

CREATE INDEX IF NOT EXISTS product_placements_dept_idx
  ON product_placements (department_id, category_id)
  WHERE active = TRUE;

CREATE INDEX IF NOT EXISTS product_placements_product_idx
  ON product_placements (product_id)
  WHERE active = TRUE;

-- Ensure at most one primary placement per product per branch (via department→branch).
CREATE UNIQUE INDEX IF NOT EXISTS product_placements_one_primary_per_branch
  ON product_placements (product_id, department_id)
  WHERE is_primary = TRUE AND active = TRUE;

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.catalog.read', 'inventory', 'read', 'catalog')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'inventory.catalog.read'
WHERE r.code IN (
  'inventory_clerk',
  'warehouse_manager',
  'regional_manager',
  'platform_admin',
  'payroll_approver'
)
ON CONFLICT DO NOTHING;

-- Demo: Sucursal Norte — Electrónica (Cómputo, Video, Telefonía, Videojuegos) + Juguetería
INSERT INTO store_departments (id, org_id, branch_id, code, name, sort_order) VALUES
  ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01', '11111111-1111-1111-1111-111111111111',
   '22222222-2222-2222-2222-222222222201', 'electronica', 'Electrónica', 10),
  ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02', '11111111-1111-1111-1111-111111111111',
   '22222222-2222-2222-2222-222222222201', 'jugueteria', 'Juguetería', 20),
  ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb11', '11111111-1111-1111-1111-111111111111',
   '22222222-2222-2222-2222-222222222202', 'electronica', 'Electrónica', 10),
  ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb12', '11111111-1111-1111-1111-111111111111',
   '22222222-2222-2222-2222-222222222202', 'jugueteria', 'Juguetería', 20)
ON CONFLICT (branch_id, code) DO NOTHING;

INSERT INTO department_categories (id, department_id, code, name, sort_order) VALUES
  ('cccccccc-cccc-cccc-cccc-cccccccccc01', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01', 'computo', 'Cómputo', 10),
  ('cccccccc-cccc-cccc-cccc-cccccccccc02', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01', 'video', 'Video', 20),
  ('cccccccc-cccc-cccc-cccc-cccccccccc03', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01', 'telefonia', 'Telefonía', 30),
  ('cccccccc-cccc-cccc-cccc-cccccccccc04', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01', 'videojuegos', 'Videojuegos', 40),
  ('cccccccc-cccc-cccc-cccc-cccccccccc11', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02', 'coleccionables', 'Coleccionables', 10),
  ('cccccccc-cccc-cccc-cccc-cccccccccc12', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02', 'juegos_mesa', 'Juegos de mesa', 20),
  ('cccccccc-cccc-cccc-cccc-ccccccccc101', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb11', 'computo', 'Cómputo', 10),
  ('cccccccc-cccc-cccc-cccc-ccccccccc104', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb11', 'videojuegos', 'Videojuegos', 40),
  ('cccccccc-cccc-cccc-cccc-ccccccccc111', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb12', 'coleccionables', 'Coleccionables', 10)
ON CONFLICT (department_id, code) DO NOTHING;

-- Retail demo products (keep existing BOLT/NUT for warehouse ops)
INSERT INTO products (id, org_id, sku_base, name) VALUES
  ('66666666-6666-6666-6666-666666666610', '11111111-1111-1111-1111-111111111111', 'LAPTOP', 'Laptop 14"'),
  ('66666666-6666-6666-6666-666666666611', '11111111-1111-1111-1111-111111111111', 'PHONE', 'Smartphone X'),
  ('66666666-6666-6666-6666-666666666612', '11111111-1111-1111-1111-111111111111', 'FIG-COL', 'Figura coleccionable ed. limitada')
ON CONFLICT (org_id, sku_base) DO NOTHING;

INSERT INTO product_skus (id, product_id, sku, uom) VALUES
  ('77777777-7777-7777-7777-777777777710', '66666666-6666-6666-6666-666666666610', 'LAPTOP-14', 'EA'),
  ('77777777-7777-7777-7777-777777777711', '66666666-6666-6666-6666-666666666611', 'PHONE-X', 'EA'),
  ('77777777-7777-7777-7777-777777777712', '66666666-6666-6666-6666-666666666612', 'FIG-COL-01', 'EA')
ON CONFLICT (sku) DO NOTHING;

INSERT INTO stock_balances (warehouse_id, sku_id, on_hand, version) VALUES
  ('55555555-5555-5555-5555-555555555501', '77777777-7777-7777-7777-777777777710', 25, 1),
  ('55555555-5555-5555-5555-555555555501', '77777777-7777-7777-7777-777777777711', 40, 1),
  ('55555555-5555-5555-5555-555555555501', '77777777-7777-7777-7777-777777777712', 15, 1),
  ('55555555-5555-5555-5555-555555555502', '77777777-7777-7777-7777-777777777710', 10, 1),
  ('55555555-5555-5555-5555-555555555502', '77777777-7777-7777-7777-777777777712', 8, 1)
ON CONFLICT (warehouse_id, sku_id) DO NOTHING;

-- Placements: laptop → Electrónica/Cómputo; phone → Electrónica/Telefonía;
-- collectible → Electrónica/Videojuegos AND Juguetería/Coleccionables (multi-dept)
INSERT INTO product_placements (product_id, department_id, category_id, is_primary) VALUES
  ('66666666-6666-6666-6666-666666666610', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01', 'cccccccc-cccc-cccc-cccc-cccccccccc01', TRUE),
  ('66666666-6666-6666-6666-666666666611', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01', 'cccccccc-cccc-cccc-cccc-cccccccccc03', TRUE),
  ('66666666-6666-6666-6666-666666666612', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01', 'cccccccc-cccc-cccc-cccc-cccccccccc04', TRUE),
  ('66666666-6666-6666-6666-666666666612', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02', 'cccccccc-cccc-cccc-cccc-cccccccccc11', FALSE),
  ('66666666-6666-6666-6666-666666666610', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb11', 'cccccccc-cccc-cccc-cccc-ccccccccc101', TRUE),
  ('66666666-6666-6666-6666-666666666612', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb11', 'cccccccc-cccc-cccc-cccc-ccccccccc104', TRUE),
  ('66666666-6666-6666-6666-666666666612', 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb12', 'cccccccc-cccc-cccc-cccc-ccccccccc111', FALSE)
ON CONFLICT (product_id, department_id, category_id) DO NOTHING;

ALTER TABLE store_departments ENABLE ROW LEVEL SECURITY;
ALTER TABLE store_departments FORCE ROW LEVEL SECURITY;
ALTER TABLE department_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE department_categories FORCE ROW LEVEL SECURITY;
ALTER TABLE product_placements ENABLE ROW LEVEL SECURITY;
ALTER TABLE product_placements FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON store_departments;
CREATE POLICY org_isolation ON store_departments
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON department_categories;
CREATE POLICY org_isolation ON department_categories
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM store_departments d
      WHERE d.id = department_categories.department_id AND d.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM store_departments d
      WHERE d.id = department_categories.department_id AND d.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON product_placements;
CREATE POLICY org_isolation ON product_placements
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM products p
      WHERE p.id = product_placements.product_id AND p.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM products p
      WHERE p.id = product_placements.product_id AND p.org_id::text = app_org_id()
    )
  );
