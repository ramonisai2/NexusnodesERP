-- 012_image_reports.sql
-- User image reports with web-optimized stored variants.

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS image_reports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  created_by TEXT NOT NULL,
  title TEXT NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  storage_key TEXT NOT NULL,
  original_filename TEXT NOT NULL DEFAULT '',
  mime_type TEXT NOT NULL,
  width INT NOT NULL CHECK (width > 0),
  height INT NOT NULL CHECK (height > 0),
  byte_size INT NOT NULL CHECK (byte_size > 0),
  original_width INT,
  original_height INT,
  original_byte_size INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS image_reports_org_branch_created_idx
  ON image_reports (org_id, branch_id, created_at DESC);

ALTER TABLE image_reports ENABLE ROW LEVEL SECURITY;
ALTER TABLE image_reports FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON image_reports;
CREATE POLICY org_isolation ON image_reports
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

INSERT INTO permissions (code, module, action, resource) VALUES
  ('reporting.image.read', 'reporting', 'read', 'image'),
  ('reporting.image.create', 'reporting', 'create', 'image')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('reporting.image.read', 'reporting.image.create')
WHERE r.code IN (
  'inventory_clerk',
  'warehouse_manager',
  'regional_manager',
  'payroll_analyst',
  'payroll_approver',
  'platform_admin'
)
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE image_reports TO nexus;
