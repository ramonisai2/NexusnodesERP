-- 013_store_setup.sql
-- First-run installation state + store_owner role for small shops.

SELECT set_config('app.rls_bypass', 'on', false);

ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS setup_completed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS profile TEXT NOT NULL DEFAULT 'enterprise'
    CHECK (profile IN ('enterprise', 'abarrotes', 'demo'));

CREATE TABLE IF NOT EXISTS app_install (
  id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  completed_at TIMESTAMPTZ,
  profile TEXT,
  store_name TEXT,
  owner_sub TEXT,
  org_id UUID REFERENCES organizations(id),
  branch_code TEXT,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO app_install (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

-- If demo org already exists (dev seed), mark install complete so SPA does not force wizard.
UPDATE app_install
SET completed_at = COALESCE(completed_at, now()),
    profile = COALESCE(profile, 'demo'),
    store_name = COALESCE(store_name, 'Nexus Demo Org'),
    owner_sub = COALESCE(owner_sub, 'usr_dev_admin'),
    org_id = COALESCE(org_id, '11111111-1111-1111-1111-111111111111'),
    branch_code = COALESCE(branch_code, 'br_norte'),
    updated_at = now()
WHERE id = 1
  AND EXISTS (SELECT 1 FROM organizations WHERE code = 'DEMO');

UPDATE organizations
SET setup_completed_at = COALESCE(setup_completed_at, now()),
    profile = CASE WHEN code = 'DEMO' THEN 'demo' ELSE profile END
WHERE code = 'DEMO';

INSERT INTO permissions (code, module, action, resource) VALUES
  ('store.setup.read', 'store', 'read', 'setup'),
  ('store.setup.manage', 'store', 'manage', 'setup')
ON CONFLICT (code) DO NOTHING;

-- store_owner role template per org (created at install time for new shops;
-- also ensure DEMO has one for local testing of the shop persona).
INSERT INTO roles (id, org_id, code, name)
SELECT '33333333-3333-3333-3333-333333333310', o.id, 'store_owner', 'Dueño de tienda'
FROM organizations o
WHERE o.code = 'DEMO'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read',
  'inventory.movement.create',
  'inventory.movement.read',
  'inventory.catalog.read',
  'inventory.label.read',
  'reporting.read',
  'reporting.image.read',
  'reporting.image.create',
  'store.setup.read'
)
WHERE r.code = 'store_owner'
ON CONFLICT DO NOTHING;
