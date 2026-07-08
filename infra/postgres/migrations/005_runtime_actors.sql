-- 005_runtime_actors.sql
-- Actors used by JWT personas + code-friendly inventory demo alignment

INSERT INTO users (id, org_id, idp_sub, email, display_name, mfa_methods) VALUES
  ('44444444-4444-4444-4444-444444444402', '11111111-1111-1111-1111-111111111111', 'usr_dev_analyst', 'analyst@demo.nexus', 'Dev Analyst', ARRAY['pwd','otp']),
  ('44444444-4444-4444-4444-444444444403', '11111111-1111-1111-1111-111111111111', 'usr_dev_approver', 'approver@demo.nexus', 'Dev Approver', ARRAY['pwd','otp']),
  ('44444444-4444-4444-4444-444444444404', '11111111-1111-1111-1111-111111111111', 'usr_dev_dual', 'dual@demo.nexus', 'Dev Dual', ARRAY['pwd','otp'])
ON CONFLICT (org_id, email) DO NOTHING;

INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, NULL
FROM users u
JOIN roles r ON r.org_id = u.org_id AND r.code = 'inventory_clerk'
WHERE u.idp_sub = 'usr_dev_analyst'
ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, NULL
FROM users u
JOIN roles r ON r.org_id = u.org_id AND r.code = 'payroll_analyst'
WHERE u.idp_sub = 'usr_dev_analyst'
ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, NULL
FROM users u
JOIN roles r ON r.org_id = u.org_id AND r.code = 'payroll_approver'
WHERE u.idp_sub = 'usr_dev_approver'
ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, NULL
FROM users u
JOIN roles r ON r.org_id = u.org_id AND r.code IN ('payroll_analyst', 'payroll_approver')
WHERE u.idp_sub = 'usr_dev_dual'
ON CONFLICT DO NOTHING;

INSERT INTO user_attributes (user_id, attr_key, attr_value)
SELECT u.id, a.attr_key, a.attr_value::jsonb
FROM users u
CROSS JOIN (VALUES
  ('max_payroll_amount', '1000000'),
  ('max_adjustment', '10000')
) AS a(attr_key, attr_value)
WHERE u.idp_sub IN ('usr_dev_analyst', 'usr_dev_approver', 'usr_dev_dual')
ON CONFLICT (user_id, attr_key) DO UPDATE SET attr_value = EXCLUDED.attr_value;

-- Extra SKU used by SPA demo alias
INSERT INTO products (id, org_id, sku_base, name)
VALUES ('66666666-6666-6666-6666-666666666602', '11111111-1111-1111-1111-111111111111', 'NUT', 'Tuerca M8')
ON CONFLICT DO NOTHING;

INSERT INTO product_skus (id, product_id, sku, uom)
VALUES ('77777777-7777-7777-7777-777777777702', '66666666-6666-6666-6666-666666666602', 'NUT-M8', 'EA')
ON CONFLICT DO NOTHING;

INSERT INTO stock_balances (warehouse_id, sku_id, on_hand, version)
VALUES
  ('55555555-5555-5555-5555-555555555501', '77777777-7777-7777-7777-777777777702', 800, 1),
  ('55555555-5555-5555-5555-555555555502', '77777777-7777-7777-7777-777777777701', 400, 1)
ON CONFLICT (warehouse_id, sku_id) DO NOTHING;
