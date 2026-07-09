-- 029_org_profiles.sql
-- Full operational profile matrix: warehouse, CEDI, dispatch, sales, cashier,
-- warranty, ecommerce, facilities, purchasing, store/regional leadership.

SELECT set_config('app.rls_bypass', 'on', false);

-- New permissions for purchasing + facilities (limpieza / mantenimiento)
INSERT INTO permissions (code, module, action, resource) VALUES
  ('purchasing.order.read', 'purchasing', 'read', 'order'),
  ('purchasing.order.create', 'purchasing', 'create', 'order'),
  ('purchasing.order.approve', 'purchasing', 'approve', 'order'),
  ('facilities.workorder.read', 'facilities', 'read', 'workorder'),
  ('facilities.workorder.create', 'facilities', 'create', 'workorder'),
  ('facilities.workorder.close', 'facilities', 'close', 'workorder')
ON CONFLICT (code) DO NOTHING;

-- Operational roles (per org)
INSERT INTO roles (id, org_id, code, name)
SELECT gen_random_uuid(), o.id, v.code, v.name
FROM organizations o
CROSS JOIN (VALUES
  ('warehouse_clerk', 'Empleado de almacén'),
  ('cedi_clerk', 'Empleado de CEDI / centro de distribución'),
  ('dispatch_clerk', 'Empleado de centro de reparto'),
  ('sales_associate', 'Vendedor de piso'),
  ('cashier', 'Cobrador / caja'),
  ('warranty_clerk', 'Garantías'),
  ('ecommerce_clerk', 'Ventas en línea'),
  ('store_coordinator', 'Coordinador de tienda'),
  ('store_admin', 'Administrador de tienda'),
  ('area_manager', 'Jefe de área'),
  ('purchasing_clerk', 'Compras (analista)'),
  ('purchasing_manager', 'Compras (gerente)'),
  ('facilities_staff', 'Limpieza / mantenimiento')
) AS v(code, name)
WHERE NOT EXISTS (
  SELECT 1 FROM roles r WHERE r.org_id = o.id AND r.code = v.code
);

-- Helper: grant a list of permission codes to a role code (all orgs)
-- Implemented as repeated INSERT…SELECT blocks below.

-- warehouse_clerk: floor warehouse ops
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.label.read',
  'inventory.warehouse.read', 'inventory.movement.create', 'inventory.movement.read',
  'inventory.movement.void.request', 'inventory.receipt.read', 'inventory.receipt.create',
  'inventory.receipt.post', 'inventory.transfer.read', 'inventory.transfer.create',
  'inventory.transfer.ship', 'inventory.transfer.receive',
  'inventory.slip.read', 'inventory.slip.create', 'inventory.slip.print',
  'inventory.slip.ship', 'inventory.slip.receive',
  'inventory.adjustment.create', 'inventory.adjustment.read',
  'session.operator', 'approval.read', 'reporting.image.read', 'reporting.image.create'
)
WHERE r.code = 'warehouse_clerk'
ON CONFLICT DO NOTHING;

-- cedi_clerk: distribution center inbound + putaway-ish transfers
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.label.read',
  'inventory.warehouse.read', 'inventory.movement.create', 'inventory.movement.read',
  'inventory.receipt.read', 'inventory.receipt.create', 'inventory.receipt.post',
  'inventory.shipment.read', 'inventory.shipment.create', 'inventory.shipment.post',
  'inventory.transfer.read', 'inventory.transfer.create', 'inventory.transfer.ship',
  'inventory.transfer.receive', 'inventory.parcel.read',
  'inventory.slip.read', 'inventory.slip.create', 'inventory.slip.print',
  'inventory.slip.ship', 'inventory.slip.receive',
  'inventory.transport.read', 'inventory.transport.create', 'inventory.transport.print',
  'inventory.transport.depart', 'inventory.transport.deliver',
  'inventory.adjustment.create', 'inventory.adjustment.read',
  'inventory.seal.verify', 'reporting.security.read',
  'session.operator', 'approval.read', 'reporting.image.read', 'reporting.image.create'
)
WHERE r.code = 'cedi_clerk'
ON CONFLICT DO NOTHING;

-- dispatch_clerk: last-mile / centro de reparto
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read',
  'inventory.parcel.read', 'inventory.slip.read', 'inventory.slip.create',
  'inventory.slip.print', 'inventory.slip.ship', 'inventory.slip.receive',
  'inventory.transport.read', 'inventory.transport.create', 'inventory.transport.print',
  'inventory.transport.depart', 'inventory.transport.deliver',
  'inventory.transfer.read', 'inventory.transfer.ship', 'inventory.transfer.receive',
  'inventory.seal.verify', 'reporting.security.read',
  'session.operator', 'reporting.image.read', 'reporting.image.create'
)
WHERE r.code = 'dispatch_clerk'
ON CONFLICT DO NOTHING;

-- sales_associate: floor seller
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.label.read',
  'customer.read', 'customer.card.read',
  'pos.sale.read', 'pos.sale.create', 'pos.invoice.request', 'pos.invoice.read',
  'session.operator', 'reporting.image.read', 'reporting.image.create'
)
WHERE r.code = 'sales_associate'
ON CONFLICT DO NOTHING;

-- cashier: cobrador
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.label.read',
  'customer.read', 'customer.card.read',
  'pos.sale.read', 'pos.sale.create', 'pos.sale.void',
  'pos.invoice.request', 'pos.invoice.read',
  'session.operator', 'approval.read'
)
WHERE r.code = 'cashier'
ON CONFLICT DO NOTHING;

-- warranty_clerk
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read',
  'inventory.warranty.read', 'inventory.warranty.create', 'inventory.warranty.manage',
  'inventory.return.read', 'inventory.return.create',
  'inventory.parcel.read', 'inventory.slip.read', 'inventory.slip.create', 'inventory.slip.print',
  'inventory.transfer.read', 'inventory.transfer.create',
  'customer.read', 'session.operator',
  'reporting.image.read', 'reporting.image.create'
)
WHERE r.code = 'warranty_clerk'
ON CONFLICT DO NOTHING;

-- ecommerce_clerk: ventas en línea
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.label.read',
  'store.storefront.read', 'store.storefront.manage',
  'customer.read', 'customer.manage', 'customer.card.read', 'customer.card.manage',
  'pos.sale.read', 'pos.invoice.read',
  'inventory.parcel.read', 'inventory.slip.read', 'inventory.slip.create', 'inventory.slip.print',
  'session.operator', 'reporting.image.read', 'reporting.image.create'
)
WHERE r.code = 'ecommerce_clerk'
ON CONFLICT DO NOTHING;

-- store_coordinator
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.label.read', 'inventory.label.manage',
  'inventory.warehouse.read', 'inventory.movement.create', 'inventory.movement.read',
  'inventory.movement.void.request', 'inventory.receipt.read', 'inventory.receipt.create', 'inventory.receipt.post',
  'inventory.transfer.read', 'inventory.transfer.create', 'inventory.transfer.ship', 'inventory.transfer.receive',
  'inventory.slip.read', 'inventory.slip.create', 'inventory.slip.print', 'inventory.slip.ship', 'inventory.slip.receive',
  'inventory.adjustment.create', 'inventory.adjustment.read',
  'inventory.parcel.read', 'inventory.warranty.read', 'inventory.return.read',
  'customer.read', 'customer.manage', 'customer.card.read', 'customer.card.manage',
  'pos.sale.read', 'pos.sale.create', 'pos.sale.void', 'pos.invoice.request', 'pos.invoice.read',
  'store.department.manager.read', 'approval.read', 'approval.decide',
  'session.operator', 'reporting.read', 'reporting.image.read', 'reporting.image.create',
  'facilities.workorder.read', 'facilities.workorder.create'
)
WHERE r.code = 'store_coordinator'
ON CONFLICT DO NOTHING;

-- store_admin: administrador de tienda (casi dueño, sin setup global)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.label.read', 'inventory.label.manage',
  'inventory.warehouse.read', 'inventory.movement.create', 'inventory.movement.read', 'inventory.movement.void',
  'inventory.movement.void.request', 'inventory.receipt.read', 'inventory.receipt.create', 'inventory.receipt.post',
  'inventory.transfer.read', 'inventory.transfer.create', 'inventory.transfer.ship', 'inventory.transfer.receive',
  'inventory.transfer.cancel', 'inventory.slip.read', 'inventory.slip.create', 'inventory.slip.print',
  'inventory.slip.ship', 'inventory.slip.receive', 'inventory.slip.cancel',
  'inventory.transport.read', 'inventory.transport.create', 'inventory.transport.print',
  'inventory.adjustment.create', 'inventory.adjustment.read',
  'inventory.parcel.read', 'inventory.warranty.read', 'inventory.warranty.manage',
  'inventory.return.read', 'inventory.return.manage',
  'customer.read', 'customer.manage', 'customer.card.read', 'customer.card.manage',
  'pos.sale.read', 'pos.sale.create', 'pos.sale.void', 'pos.invoice.request', 'pos.invoice.read', 'pos.settings.manage',
  'store.storefront.read', 'store.storefront.manage',
  'store.department.manager.read', 'store.department.manager.assign',
  'employee.read', 'approval.read', 'approval.decide',
  'session.operator', 'reporting.read', 'reporting.image.read', 'reporting.image.create',
  'reporting.security.read', 'inventory.seal.verify',
  'facilities.workorder.read', 'facilities.workorder.create', 'facilities.workorder.close',
  'purchasing.order.read'
)
WHERE r.code = 'store_admin'
ON CONFLICT DO NOTHING;

-- area_manager: jefe de área (similar warehouse_manager)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.label.read',
  'inventory.warehouse.read', 'inventory.movement.create', 'inventory.movement.read', 'inventory.movement.void',
  'inventory.receipt.read', 'inventory.receipt.create', 'inventory.receipt.post',
  'inventory.transfer.read', 'inventory.transfer.create', 'inventory.transfer.ship',
  'inventory.transfer.receive', 'inventory.transfer.cancel',
  'inventory.slip.read', 'inventory.slip.create', 'inventory.slip.print', 'inventory.slip.cancel',
  'inventory.transport.read', 'inventory.transport.create', 'inventory.transport.print', 'inventory.transport.cancel',
  'inventory.adjustment.create', 'inventory.adjustment.read',
  'inventory.parcel.read', 'inventory.shipment.read',
  'approval.read', 'approval.decide', 'session.operator',
  'reporting.read', 'reporting.image.read', 'reporting.image.create',
  'reporting.security.read', 'inventory.seal.verify',
  'store.department.manager.read', 'pos.sale.read', 'pos.sale.create'
)
WHERE r.code = 'area_manager'
ON CONFLICT DO NOTHING;

-- purchasing_clerk
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'purchasing.order.read', 'purchasing.order.create',
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.warehouse.read',
  'inventory.receipt.read', 'inventory.shipment.read',
  'session.operator', 'reporting.read', 'approval.read'
)
WHERE r.code = 'purchasing_clerk'
ON CONFLICT DO NOTHING;

-- purchasing_manager
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'purchasing.order.read', 'purchasing.order.create', 'purchasing.order.approve',
  'inventory.balance.read', 'inventory.catalog.read', 'inventory.warehouse.read',
  'inventory.receipt.read', 'inventory.receipt.create', 'inventory.shipment.read',
  'inventory.shipment.create', 'approval.read', 'approval.decide',
  'session.operator', 'reporting.read'
)
WHERE r.code = 'purchasing_manager'
ON CONFLICT DO NOTHING;

-- facilities_staff: limpieza / mantenimiento
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'facilities.workorder.read', 'facilities.workorder.create', 'facilities.workorder.close',
  'reporting.image.read', 'reporting.image.create',
  'session.operator', 'inventory.balance.read'
)
WHERE r.code = 'facilities_staff'
ON CONFLICT DO NOTHING;

-- Also grant purchasing/facilities visibility to existing leadership roles
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'purchasing.order.read', 'purchasing.order.create', 'purchasing.order.approve',
  'facilities.workorder.read', 'facilities.workorder.create', 'facilities.workorder.close'
)
WHERE r.code IN ('platform_admin', 'regional_manager', 'store_owner')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'purchasing.order.read',
  'facilities.workorder.read', 'facilities.workorder.create', 'facilities.workorder.close'
)
WHERE r.code IN ('warehouse_manager')
ON CONFLICT DO NOTHING;

-- Demo users for each new profile (DEMO org)
INSERT INTO users (id, org_id, idp_sub, email, display_name, mfa_methods)
SELECT v.id::uuid, o.id, v.idp_sub, v.email, v.display_name, ARRAY['pwd','otp']
FROM organizations o
CROSS JOIN (VALUES
  ('44444444-4444-4444-4444-444444444501', 'usr_dev_warehouse', 'warehouse@demo.nexus', 'Dev Almacén'),
  ('44444444-4444-4444-4444-444444444502', 'usr_dev_cedi', 'cedi@demo.nexus', 'Dev CEDI'),
  ('44444444-4444-4444-4444-444444444503', 'usr_dev_dispatch', 'dispatch@demo.nexus', 'Dev Reparto'),
  ('44444444-4444-4444-4444-444444444504', 'usr_dev_sales', 'sales@demo.nexus', 'Dev Vendedor'),
  ('44444444-4444-4444-4444-444444444505', 'usr_dev_cashier', 'cashier@demo.nexus', 'Dev Cobrador'),
  ('44444444-4444-4444-4444-444444444506', 'usr_dev_warranty', 'warranty@demo.nexus', 'Dev Garantías'),
  ('44444444-4444-4444-4444-444444444507', 'usr_dev_ecommerce', 'ecommerce@demo.nexus', 'Dev Ventas en línea'),
  ('44444444-4444-4444-4444-444444444508', 'usr_dev_coordinator', 'coordinator@demo.nexus', 'Dev Coordinador tienda'),
  ('44444444-4444-4444-4444-444444444509', 'usr_dev_store_admin', 'storeadmin@demo.nexus', 'Dev Admin tienda'),
  ('44444444-4444-4444-4444-444444444510', 'usr_dev_area', 'area@demo.nexus', 'Dev Jefe de área'),
  ('44444444-4444-4444-4444-444444444511', 'usr_dev_purchasing', 'purchasing@demo.nexus', 'Dev Compras'),
  ('44444444-4444-4444-4444-444444444512', 'usr_dev_purchasing_mgr', 'purchasing.mgr@demo.nexus', 'Dev Gerente compras'),
  ('44444444-4444-4444-4444-444444444513', 'usr_dev_facilities', 'facilities@demo.nexus', 'Dev Limpieza/Mantenimiento'),
  ('44444444-4444-4444-4444-444444444514', 'usr_dev_hr', 'hr@demo.nexus', 'Dev Recursos Humanos')
) AS v(id, idp_sub, email, display_name)
WHERE o.code = 'DEMO'
ON CONFLICT (idp_sub) DO NOTHING;

-- Assign roles to demo users on relevant branches
INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, b.id
FROM users u
JOIN roles r ON r.org_id = u.org_id
JOIN branches b ON b.org_id = u.org_id
JOIN (VALUES
  ('usr_dev_warehouse', 'warehouse_clerk', 'br_norte'),
  ('usr_dev_cedi', 'cedi_clerk', 'br_cedi'),
  ('usr_dev_dispatch', 'dispatch_clerk', 'br_cedi'),
  ('usr_dev_dispatch', 'dispatch_clerk', 'br_norte'),
  ('usr_dev_sales', 'sales_associate', 'br_norte'),
  ('usr_dev_cashier', 'cashier', 'br_norte'),
  ('usr_dev_warranty', 'warranty_clerk', 'br_norte'),
  ('usr_dev_ecommerce', 'ecommerce_clerk', 'br_norte'),
  ('usr_dev_coordinator', 'store_coordinator', 'br_norte'),
  ('usr_dev_store_admin', 'store_admin', 'br_norte'),
  ('usr_dev_area', 'area_manager', 'br_norte'),
  ('usr_dev_purchasing', 'purchasing_clerk', 'br_cedi'),
  ('usr_dev_purchasing', 'purchasing_clerk', 'br_norte'),
  ('usr_dev_purchasing_mgr', 'purchasing_manager', 'br_cedi'),
  ('usr_dev_purchasing_mgr', 'purchasing_manager', 'br_norte'),
  ('usr_dev_facilities', 'facilities_staff', 'br_norte'),
  ('usr_dev_facilities', 'facilities_staff', 'br_cedi'),
  ('usr_dev_hr', 'hr_officer', 'br_norte'),
  ('usr_dev_hr', 'hr_officer', 'br_sur')
) AS m(idp_sub, role_code, branch_code)
  ON u.idp_sub = m.idp_sub AND r.code = m.role_code AND b.code = m.branch_code
ON CONFLICT DO NOTHING;
