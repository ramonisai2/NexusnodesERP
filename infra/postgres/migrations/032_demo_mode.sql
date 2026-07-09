-- 032_demo_mode.sql
-- Guided demo explorer user for DEMO org (all modules unlocked).

SELECT set_config('app.rls_bypass', 'on', false);

-- Keep DEMO org marked as demo profile and fully unlocked.
UPDATE organizations
SET profile = 'demo',
    enabled_modules = NULL
WHERE code = 'DEMO';

INSERT INTO users (id, org_id, idp_sub, email, display_name, mfa_methods)
SELECT '44444444-4444-4444-4444-444444444530'::uuid, o.id,
       'usr_dev_demo', 'demo@demo.nexus', 'Explorador Demo',
       ARRAY['pwd','otp']
FROM organizations o
WHERE o.code = 'DEMO'
ON CONFLICT (idp_sub) DO NOTHING;

-- Broad tour: store admin + HR + security + webmaster on main branches
INSERT INTO user_roles (user_id, role_id, branch_id)
SELECT u.id, r.id, b.id
FROM users u
JOIN roles r ON r.org_id = u.org_id
JOIN branches b ON b.org_id = u.org_id
JOIN (VALUES
  ('usr_dev_demo', 'store_admin', 'br_norte'),
  ('usr_dev_demo', 'store_admin', 'br_sur'),
  ('usr_dev_demo', 'store_admin', 'br_cedi'),
  ('usr_dev_demo', 'hr_officer', 'br_norte'),
  ('usr_dev_demo', 'hr_officer', 'br_sur'),
  ('usr_dev_demo', 'security_officer', 'br_norte'),
  ('usr_dev_demo', 'security_officer', 'br_cedi'),
  ('usr_dev_demo', 'webmaster', 'br_norte'),
  ('usr_dev_demo', 'webmaster', 'br_sur'),
  ('usr_dev_demo', 'webmaster', 'br_cedi')
) AS m(idp_sub, role_code, branch_code)
  ON u.idp_sub = m.idp_sub AND r.code = m.role_code AND b.code = m.branch_code
ON CONFLICT DO NOTHING;

INSERT INTO user_attributes (user_id, attr_key, attr_value)
SELECT u.id, v.k, v.v::jsonb
FROM users u
CROSS JOIN (VALUES
  ('demo_mode', 'true'),
  ('profile', '"demo"'),
  ('max_adjustment', '100000')
) AS v(k, v)
WHERE u.idp_sub = 'usr_dev_demo'
ON CONFLICT (user_id, attr_key) DO UPDATE SET attr_value = EXCLUDED.attr_value;
