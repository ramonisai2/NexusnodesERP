-- 027_merma_robo_hr.sql
-- Shrink/theft reason codes on inventory movements + HR employee permissions.

SELECT set_config('app.rls_bypass', 'on', false);

ALTER TABLE inventory_movements
  ADD COLUMN IF NOT EXISTS reason_code TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';

ALTER TABLE inventory_movements
  DROP CONSTRAINT IF EXISTS inventory_movements_reason_code_check;

ALTER TABLE inventory_movements
  ADD CONSTRAINT inventory_movements_reason_code_check
  CHECK (
    reason_code = '' OR reason_code IN (
      'MERMA', 'ROBO', 'DAMAGE', 'EXPIRED', 'COUNT_VARIANCE', 'FOUND', 'OTHER'
    )
  );

CREATE INDEX IF NOT EXISTS inventory_movements_reason_idx
  ON inventory_movements (org_id, reason_code)
  WHERE reason_code <> '';

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.adjustment.create', 'inventory', 'create', 'adjustment'),
  ('inventory.adjustment.read', 'inventory', 'read', 'adjustment'),
  ('employee.read', 'hr', 'read', 'employee'),
  ('employee.write', 'hr', 'write', 'employee')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.adjustment.create', 'inventory.adjustment.read',
  'inventory.movement.create', 'inventory.movement.read'
)
WHERE r.code IN (
  'inventory_clerk', 'warehouse_manager', 'regional_manager',
  'store_owner', 'platform_admin'
)
ON CONFLICT DO NOTHING;

INSERT INTO roles (id, org_id, code, name)
SELECT gen_random_uuid(), o.id, 'hr_officer', 'Recursos Humanos'
FROM organizations o
WHERE NOT EXISTS (
  SELECT 1 FROM roles r WHERE r.org_id = o.id AND r.code = 'hr_officer'
);

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'employee.read', 'employee.write',
  'payroll.run.read', 'payroll.run.prepare'
)
WHERE r.code IN ('hr_officer', 'payroll_analyst', 'payroll_approver', 'platform_admin', 'regional_manager')
ON CONFLICT DO NOTHING;

-- Analysts / clerks who already run payroll can also list employees.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'employee.read'
WHERE r.code IN ('inventory_clerk', 'warehouse_manager', 'store_owner')
ON CONFLICT DO NOTHING;
