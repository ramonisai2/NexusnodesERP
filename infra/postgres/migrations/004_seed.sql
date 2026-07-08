-- 004_seed.sql
-- Demo tenant for local development

INSERT INTO organizations (id, code, name)
VALUES ('11111111-1111-1111-1111-111111111111', 'DEMO', 'Nexus Demo Org');

INSERT INTO branches (id, org_id, code, name, region) VALUES
  ('22222222-2222-2222-2222-222222222201', '11111111-1111-1111-1111-111111111111', 'br_norte', 'Sucursal Norte', 'NORTE'),
  ('22222222-2222-2222-2222-222222222202', '11111111-1111-1111-1111-111111111111', 'br_sur', 'Sucursal Sur', 'SUR');

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.movement.create', 'inventory', 'create', 'movement'),
  ('inventory.balance.read', 'inventory', 'read', 'balance'),
  ('payroll.run.prepare', 'payroll', 'prepare', 'run'),
  ('payroll.run.approve', 'payroll', 'approve', 'run'),
  ('payroll.run.read', 'payroll', 'read', 'run');

INSERT INTO roles (id, org_id, code, name) VALUES
  ('33333333-3333-3333-3333-333333333301', '11111111-1111-1111-1111-111111111111', 'inventory_clerk', 'Inventory Clerk'),
  ('33333333-3333-3333-3333-333333333302', '11111111-1111-1111-1111-111111111111', 'payroll_analyst', 'Payroll Analyst'),
  ('33333333-3333-3333-3333-333333333303', '11111111-1111-1111-1111-111111111111', 'payroll_approver', 'Payroll Approver'),
  ('33333333-3333-3333-3333-333333333304', '11111111-1111-1111-1111-111111111111', 'platform_admin', 'Platform Admin');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.code = 'platform_admin';

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.code IN ('inventory.movement.create', 'inventory.balance.read')
WHERE r.code = 'inventory_clerk';

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.code IN ('payroll.run.prepare', 'payroll.run.read')
WHERE r.code = 'payroll_analyst';

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.code IN ('payroll.run.approve', 'payroll.run.read')
WHERE r.code = 'payroll_approver';

INSERT INTO users (id, org_id, idp_sub, email, display_name, mfa_methods) VALUES
  ('44444444-4444-4444-4444-444444444401', '11111111-1111-1111-1111-111111111111', 'usr_dev_admin', 'admin@demo.nexus', 'Dev Admin', ARRAY['pwd','otp']);

INSERT INTO user_roles (user_id, role_id, branch_id)
VALUES ('44444444-4444-4444-4444-444444444401', '33333333-3333-3333-3333-333333333304', NULL);

INSERT INTO user_attributes (user_id, attr_key, attr_value) VALUES
  ('44444444-4444-4444-4444-444444444401', 'max_payroll_amount', '1000000'),
  ('44444444-4444-4444-4444-444444444401', 'max_adjustment', '10000');

INSERT INTO warehouses (id, branch_id, code, name) VALUES
  ('55555555-5555-5555-5555-555555555501', '22222222-2222-2222-2222-222222222201', 'wh_norte', 'Almacén Norte'),
  ('55555555-5555-5555-5555-555555555502', '22222222-2222-2222-2222-222222222202', 'wh_sur', 'Almacén Sur');

INSERT INTO products (id, org_id, sku_base, name)
VALUES ('66666666-6666-6666-6666-666666666601', '11111111-1111-1111-1111-111111111111', 'BOLT', 'Tornillo M8');

INSERT INTO product_skus (id, product_id, sku, uom)
VALUES ('77777777-7777-7777-7777-777777777701', '66666666-6666-6666-6666-666666666601', 'BOLT-M8', 'EA');

INSERT INTO stock_balances (warehouse_id, sku_id, on_hand, version) VALUES
  ('55555555-5555-5555-5555-555555555501', '77777777-7777-7777-7777-777777777701', 1000, 1),
  ('55555555-5555-5555-5555-555555555502', '77777777-7777-7777-7777-777777777701', 400, 1);

INSERT INTO employees (id, org_id, branch_id, employee_number, display_name) VALUES
  ('88888888-8888-8888-8888-888888888801', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222201', 'E-001', 'Ana López'),
  ('88888888-8888-8888-8888-888888888802', '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222201', 'E-002', 'Luis Pérez');

INSERT INTO payroll_concepts (org_id, code, name, concept_type) VALUES
  ('11111111-1111-1111-1111-111111111111', 'BASE', 'Sueldo base', 'EARNING');

INSERT INTO payroll_periods (org_id, label, start_date, end_date)
VALUES ('11111111-1111-1111-1111-111111111111', '2026-07-H1', '2026-07-01', '2026-07-15');
