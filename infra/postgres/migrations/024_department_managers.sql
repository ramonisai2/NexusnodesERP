-- 024_department_managers.sql
-- A store manager (jefe) may own several store departments with no relation required
-- between them (e.g. Electrónica + Juguetería). Scope is independent of org_units tree.

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS department_managers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  department_id UUID NOT NULL REFERENCES store_departments(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  valid_to TIMESTAMPTZ,
  assigned_by UUID REFERENCES users(id),
  UNIQUE (department_id, user_id)
);

CREATE INDEX IF NOT EXISTS department_managers_user_idx
  ON department_managers (user_id)
  WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS department_managers_dept_idx
  ON department_managers (department_id)
  WHERE valid_to IS NULL;

ALTER TABLE department_managers ENABLE ROW LEVEL SECURITY;
ALTER TABLE department_managers FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON department_managers;
CREATE POLICY org_isolation ON department_managers
  USING (
    current_setting('app.rls_bypass', true) = 'on'
    OR org_id::text = NULLIF(current_setting('app.org_id', true), '')
  )
  WITH CHECK (
    current_setting('app.rls_bypass', true) = 'on'
    OR org_id::text = NULLIF(current_setting('app.org_id', true), '')
  );

INSERT INTO permissions (code, module, action, resource) VALUES
  ('store.department.manager.read', 'store', 'read', 'department_manager'),
  ('store.department.manager.assign', 'store', 'assign', 'department_manager')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'store.department.manager.read',
  'store.department.manager.assign'
)
WHERE r.code IN ('store_owner', 'platform_admin', 'regional_manager')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'store.department.manager.read'
WHERE r.code IN ('warehouse_manager')
ON CONFLICT DO NOTHING;

-- Demo: jefe de almacén Norte también es responsable de Electrónica y Juguetería
-- (departamentos no relacionados entre sí).
INSERT INTO department_managers (org_id, department_id, user_id)
SELECT
  sd.org_id,
  sd.id,
  u.id
FROM store_departments sd
JOIN users u ON u.idp_sub = 'usr_dev_wh_manager' AND u.org_id = sd.org_id
JOIN branches b ON b.id = sd.branch_id AND b.code = 'br_norte'
WHERE sd.code IN ('electronica', 'jugueteria')
ON CONFLICT (department_id, user_id) DO NOTHING;
