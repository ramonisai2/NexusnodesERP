-- 030_webmaster_reports.sql
-- Webmaster role + export/print permissions for digital and printed reports
-- across almost all administered domains.

SELECT set_config('app.rls_bypass', 'on', false);

INSERT INTO permissions (code, module, action, resource) VALUES
  ('reporting.export', 'reporting', 'export', 'report'),
  ('reporting.print', 'reporting', 'print', 'report'),
  ('reporting.catalog.read', 'reporting', 'read', 'catalog')
ON CONFLICT (code) DO NOTHING;

-- Webmaster: broad read + export/print across administered domains
INSERT INTO roles (id, org_id, code, name)
SELECT gen_random_uuid(), o.id, 'webmaster', 'Webmaster / reportes'
FROM organizations o
WHERE NOT EXISTS (
  SELECT 1 FROM roles r WHERE r.org_id = o.id AND r.code = 'webmaster'
);

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  -- reporting hub
  'reporting.read', 'reporting.export', 'reporting.print', 'reporting.catalog.read',
  'reporting.image.read', 'reporting.security.read',
  -- inventory reads
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.label.read',
  'inventory.warehouse.read', 'inventory.movement.read',
  'inventory.receipt.read', 'inventory.shipment.read',
  'inventory.transfer.read', 'inventory.slip.read', 'inventory.transport.read',
  'inventory.parcel.read', 'inventory.warranty.read', 'inventory.return.read',
  'inventory.adjustment.read', 'inventory.seal.verify',
  -- POS / customers
  'pos.sale.read', 'pos.invoice.read', 'customer.read', 'customer.card.read',
  -- people / payroll
  'employee.read', 'payroll.run.read',
  -- store / purchasing / facilities visibility
  'store.storefront.read', 'store.department.manager.read',
  'purchasing.order.read', 'facilities.workorder.read',
  'approval.read', 'session.operator'
)
WHERE r.code = 'webmaster'
ON CONFLICT DO NOTHING;

-- Leadership also gets export/print
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'reporting.export', 'reporting.print', 'reporting.catalog.read'
)
WHERE r.code IN (
  'platform_admin', 'store_owner', 'store_admin', 'regional_manager',
  'area_manager', 'store_coordinator'
)
ON CONFLICT DO NOTHING;

-- Demo webmaster user
INSERT INTO users (id, org_id, idp_sub, email, display_name, mfa_methods)
SELECT '44444444-4444-4444-4444-444444444520'::uuid, o.id,
       'usr_dev_webmaster', 'webmaster@demo.nexus', 'Dev Webmaster',
       ARRAY['pwd','otp']
FROM organizations o
WHERE o.code = 'DEMO'
ON CONFLICT (idp_sub) DO NOTHING;

INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, b.id
FROM users u
JOIN roles r ON r.org_id = u.org_id AND r.code = 'webmaster'
JOIN branches b ON b.org_id = u.org_id
WHERE u.idp_sub = 'usr_dev_webmaster'
  AND b.code IN ('br_norte', 'br_sur', 'br_cedi')
ON CONFLICT DO NOTHING;
