-- 031_install_modules.sql
-- Org-level enabled modules for first-use install lock/unlock.

SELECT set_config('app.rls_bypass', 'on', false);

ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS enabled_modules JSONB;

COMMENT ON COLUMN organizations.enabled_modules IS
  'JSON array of module codes unlocked at install. NULL = all unlocked (legacy/enterprise).';

-- Ensure store.setup.manage exists for post-install module edits
INSERT INTO permissions (code, module, action, resource) VALUES
  ('store.setup.manage', 'store', 'manage', 'setup')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'store.setup.manage'
WHERE r.code IN ('store_owner', 'store_admin', 'platform_admin', 'webmaster')
ON CONFLICT DO NOTHING;

-- DEMO org stays fully unlocked (NULL)
UPDATE organizations SET enabled_modules = NULL WHERE code = 'DEMO' AND enabled_modules IS NULL;
