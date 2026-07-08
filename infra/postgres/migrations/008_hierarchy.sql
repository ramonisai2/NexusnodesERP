-- 008_hierarchy.sql
-- Organizational hierarchy for area managers and movement reversals.

CREATE TABLE org_units (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  parent_id UUID REFERENCES org_units(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  unit_type TEXT NOT NULL DEFAULT 'AREA', -- AREA | REGION | DIVISION
  active BOOLEAN NOT NULL DEFAULT TRUE,
  UNIQUE (org_id, code)
);

CREATE INDEX org_units_parent_idx ON org_units (parent_id);

-- Who manages which unit (and by inheritance, child units / warehouses).
CREATE TABLE org_unit_managers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_unit_id UUID NOT NULL REFERENCES org_units(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  manager_level TEXT NOT NULL, -- AREA_MANAGER | REGIONAL_MANAGER
  valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  valid_to TIMESTAMPTZ,
  UNIQUE (org_unit_id, user_id, manager_level)
);

CREATE INDEX org_unit_managers_user_idx ON org_unit_managers (user_id);

ALTER TABLE warehouses
  ADD COLUMN IF NOT EXISTS org_unit_id UUID REFERENCES org_units(id);

ALTER TABLE inventory_movements
  ADD COLUMN IF NOT EXISTS reversal_of UUID REFERENCES inventory_movements(id),
  ADD COLUMN IF NOT EXISTS void_reason TEXT,
  ADD COLUMN IF NOT EXISTS voided_by UUID REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS voided_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS inventory_movements_warehouse_created_idx
  ON inventory_movements (warehouse_id, created_at DESC);

-- Seed / DDL that touches RLS-protected tables needs bypass (same pattern as migrate tooling).
SELECT set_config('app.rls_bypass', 'on', false);

-- Permissions & roles
INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.movement.read', 'inventory', 'read', 'movement'),
  ('inventory.movement.void', 'inventory', 'void', 'movement')
ON CONFLICT (code) DO NOTHING;

INSERT INTO roles (id, org_id, code, name) VALUES
  ('33333333-3333-3333-3333-333333333305', '11111111-1111-1111-1111-111111111111', 'warehouse_manager', 'Jefe de almacén / área'),
  ('33333333-3333-3333-3333-333333333306', '11111111-1111-1111-1111-111111111111', 'regional_manager', 'Jefe regional (superior)')
ON CONFLICT (org_id, code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read',
  'inventory.movement.read',
  'inventory.movement.create',
  'inventory.movement.void'
)
WHERE r.code = 'warehouse_manager'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read',
  'inventory.movement.read',
  'inventory.movement.create',
  'inventory.movement.void',
  'payroll.run.read'
)
WHERE r.code = 'regional_manager'
ON CONFLICT DO NOTHING;

-- Also grant void/read to platform_admin via existing cross join? platform_admin already has all from seed.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r
JOIN permissions p ON p.code IN ('inventory.movement.read', 'inventory.movement.void')
WHERE r.code = 'platform_admin'
ON CONFLICT DO NOTHING;

-- Demo hierarchy: Región Norte → Área Almacén Norte; Región Sur → Área Almacén Sur
INSERT INTO org_units (id, org_id, parent_id, code, name, unit_type) VALUES
  ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01', '11111111-1111-1111-1111-111111111111', NULL, 'region_norte', 'Región Norte', 'REGION'),
  ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa02', '11111111-1111-1111-1111-111111111111', NULL, 'region_sur', 'Región Sur', 'REGION'),
  ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa11', '11111111-1111-1111-1111-111111111111', 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01', 'area_wh_norte', 'Área Almacén Norte', 'AREA'),
  ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa12', '11111111-1111-1111-1111-111111111111', 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa02', 'area_wh_sur', 'Área Almacén Sur', 'AREA')
ON CONFLICT (org_id, code) DO NOTHING;

UPDATE warehouses SET org_unit_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa11'
WHERE code = 'wh_norte' AND org_unit_id IS NULL;

UPDATE warehouses SET org_unit_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa12'
WHERE code = 'wh_sur' AND org_unit_id IS NULL;

-- Manager users
INSERT INTO users (id, org_id, idp_sub, email, display_name, mfa_methods) VALUES
  ('44444444-4444-4444-4444-444444444411', '11111111-1111-1111-1111-111111111111', 'usr_dev_wh_manager', 'wh.manager@demo.nexus', 'Jefe Almacén Norte', ARRAY['pwd','otp']),
  ('44444444-4444-4444-4444-444444444412', '11111111-1111-1111-1111-111111111111', 'usr_dev_regional', 'regional@demo.nexus', 'Jefe Regional Norte', ARRAY['pwd','otp'])
ON CONFLICT (org_id, email) DO NOTHING;

INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, '22222222-2222-2222-2222-222222222201'
FROM users u
JOIN roles r ON r.org_id = u.org_id AND r.code = 'warehouse_manager'
WHERE u.idp_sub = 'usr_dev_wh_manager'
ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, NULL
FROM users u
JOIN roles r ON r.org_id = u.org_id AND r.code = 'regional_manager'
WHERE u.idp_sub = 'usr_dev_regional'
ON CONFLICT DO NOTHING;

INSERT INTO org_unit_managers (org_unit_id, user_id, manager_level)
SELECT 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa11', u.id, 'AREA_MANAGER'
FROM users u WHERE u.idp_sub = 'usr_dev_wh_manager'
ON CONFLICT DO NOTHING;

INSERT INTO org_unit_managers (org_unit_id, user_id, manager_level)
SELECT 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01', u.id, 'REGIONAL_MANAGER'
FROM users u WHERE u.idp_sub = 'usr_dev_regional'
ON CONFLICT DO NOTHING;

INSERT INTO user_attributes (user_id, attr_key, attr_value)
SELECT u.id, a.attr_key, a.attr_value::jsonb
FROM users u
CROSS JOIN (VALUES
  ('max_adjustment', '50000'),
  ('max_payroll_amount', '0')
) AS a(attr_key, attr_value)
WHERE u.idp_sub IN ('usr_dev_wh_manager', 'usr_dev_regional')
ON CONFLICT (user_id, attr_key) DO UPDATE SET attr_value = EXCLUDED.attr_value;

-- RLS for new tables
ALTER TABLE org_units ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_units FORCE ROW LEVEL SECURITY;
ALTER TABLE org_unit_managers ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_unit_managers FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON org_units;
CREATE POLICY org_isolation ON org_units
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON org_unit_managers;
CREATE POLICY org_isolation ON org_unit_managers
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM org_units ou
      WHERE ou.id = org_unit_managers.org_unit_id AND ou.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM org_units ou
      WHERE ou.id = org_unit_managers.org_unit_id AND ou.org_id::text = app_org_id()
    )
  );
