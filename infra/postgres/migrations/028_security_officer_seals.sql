-- 028_security_officer_seals.sql
-- Physical security staff: read-only logistics reports + container seal tracking.

SELECT set_config('app.rls_bypass', 'on', false);

-- Seals on outbound transport sheets (hoja de transporte / unidad)
ALTER TABLE transport_sheets
  ADD COLUMN IF NOT EXISTS seal_number TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS seal_status TEXT NOT NULL DEFAULT ''
    CHECK (seal_status = '' OR seal_status IN ('APPLIED', 'VERIFIED', 'BROKEN', 'MISSING')),
  ADD COLUMN IF NOT EXISTS seal_verified_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS seal_verified_by UUID REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS seal_notes TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS transport_sheets_seal_idx
  ON transport_sheets (org_id, seal_number)
  WHERE seal_number <> '';

-- Seals on inbound supplier trucks at CEDI
ALTER TABLE inbound_shipments
  ADD COLUMN IF NOT EXISTS seal_number TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS seal_status TEXT NOT NULL DEFAULT ''
    CHECK (seal_status = '' OR seal_status IN ('APPLIED', 'VERIFIED', 'BROKEN', 'MISSING')),
  ADD COLUMN IF NOT EXISTS seal_verified_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS seal_verified_by UUID REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS seal_notes TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS inbound_shipments_seal_idx
  ON inbound_shipments (org_id, seal_number)
  WHERE seal_number <> '';

INSERT INTO permissions (code, module, action, resource) VALUES
  ('reporting.security.read', 'reporting', 'read', 'security'),
  ('inventory.seal.verify', 'inventory', 'verify', 'seal')
ON CONFLICT (code) DO NOTHING;

INSERT INTO roles (id, org_id, code, name)
SELECT gen_random_uuid(), o.id, 'security_officer', 'Seguridad / vigilancia'
FROM organizations o
WHERE NOT EXISTS (
  SELECT 1 FROM roles r WHERE r.org_id = o.id AND r.code = 'security_officer'
);

-- Security: read logistics + verify seals (no stock mutations)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'reporting.security.read',
  'inventory.seal.verify',
  'inventory.slip.read',
  'inventory.transport.read',
  'inventory.parcel.read',
  'inventory.transfer.read',
  'inventory.shipment.read',
  'inventory.balance.read',
  'reporting.image.read',
  'reporting.image.create'
)
WHERE r.code = 'security_officer'
ON CONFLICT DO NOTHING;

-- Warehouse / regional managers can also verify seals and open the security report
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'reporting.security.read',
  'inventory.seal.verify'
)
WHERE r.code IN (
  'warehouse_manager', 'regional_manager', 'platform_admin', 'store_owner', 'inventory_clerk'
)
ON CONFLICT DO NOTHING;

-- Dev security officer user (demo persona)
INSERT INTO users (id, org_id, idp_sub, email, display_name, mfa_methods)
SELECT '44444444-4444-4444-4444-444444444490', o.id, 'usr_dev_security', 'security@demo.nexus', 'Dev Seguridad', ARRAY['pwd','otp']
FROM organizations o
WHERE o.code = 'DEMO'
ON CONFLICT (idp_sub) DO NOTHING;

INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, b.id
FROM users u
JOIN roles r ON r.org_id = u.org_id AND r.code = 'security_officer'
JOIN branches b ON b.org_id = u.org_id AND b.code IN ('br_norte', 'br_sur', 'br_cedi')
WHERE u.idp_sub = 'usr_dev_security'
ON CONFLICT DO NOTHING;
