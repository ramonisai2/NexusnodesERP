-- 006_rls.sql
-- Row-Level Security by organization. App must SET app.org_id (UUID) per request.
-- Optional bypass: SET app.rls_bypass = 'on' (admin/migrate only).

ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
ALTER TABLE branches ENABLE ROW LEVEL SECURITY;
ALTER TABLE branches FORCE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE roles FORCE ROW LEVEL SECURITY;
ALTER TABLE user_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_roles FORCE ROW LEVEL SECURITY;
ALTER TABLE user_attributes ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_attributes FORCE ROW LEVEL SECURITY;
ALTER TABLE warehouses ENABLE ROW LEVEL SECURITY;
ALTER TABLE warehouses FORCE ROW LEVEL SECURITY;
ALTER TABLE products ENABLE ROW LEVEL SECURITY;
ALTER TABLE products FORCE ROW LEVEL SECURITY;
ALTER TABLE product_skus ENABLE ROW LEVEL SECURITY;
ALTER TABLE product_skus FORCE ROW LEVEL SECURITY;
ALTER TABLE stock_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_balances FORCE ROW LEVEL SECURITY;
ALTER TABLE inventory_movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_movements FORCE ROW LEVEL SECURITY;
ALTER TABLE stock_reservations ENABLE ROW LEVEL SECURITY;
ALTER TABLE stock_reservations FORCE ROW LEVEL SECURITY;
ALTER TABLE employees ENABLE ROW LEVEL SECURITY;
ALTER TABLE employees FORCE ROW LEVEL SECURITY;
ALTER TABLE employment_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE employment_contracts FORCE ROW LEVEL SECURITY;
ALTER TABLE payroll_periods ENABLE ROW LEVEL SECURITY;
ALTER TABLE payroll_periods FORCE ROW LEVEL SECURITY;
ALTER TABLE payroll_concepts ENABLE ROW LEVEL SECURITY;
ALTER TABLE payroll_concepts FORCE ROW LEVEL SECURITY;
ALTER TABLE payroll_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE payroll_runs FORCE ROW LEVEL SECURITY;
ALTER TABLE payroll_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE payroll_lines FORCE ROW LEVEL SECURITY;
ALTER TABLE payroll_approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE payroll_approvals FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;
-- outbox is operational; keep readable by relay without org context
-- sessions/token_denylist/permissions/policies remain global

CREATE OR REPLACE FUNCTION app_rls_bypass()
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
  SELECT COALESCE(current_setting('app.rls_bypass', true), '') = 'on'
$$;

CREATE OR REPLACE FUNCTION app_org_id()
RETURNS text
LANGUAGE sql
STABLE
AS $$
  SELECT NULLIF(current_setting('app.org_id', true), '')
$$;

-- Helper: org match
CREATE OR REPLACE FUNCTION app_org_matches(target uuid)
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
  SELECT app_rls_bypass() OR (app_org_id() IS NOT NULL AND target::text = app_org_id())
$$;

DROP POLICY IF EXISTS org_isolation ON organizations;
CREATE POLICY org_isolation ON organizations
  FOR ALL
  USING (app_rls_bypass() OR id::text = app_org_id())
  WITH CHECK (app_rls_bypass() OR id::text = app_org_id());

DROP POLICY IF EXISTS org_isolation ON branches;
CREATE POLICY org_isolation ON branches
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON users;
CREATE POLICY org_isolation ON users
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON roles;
CREATE POLICY org_isolation ON roles
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON user_roles;
CREATE POLICY org_isolation ON user_roles
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM users u WHERE u.id = user_roles.user_id AND u.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM users u WHERE u.id = user_roles.user_id AND u.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON user_attributes;
CREATE POLICY org_isolation ON user_attributes
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM users u WHERE u.id = user_attributes.user_id AND u.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM users u WHERE u.id = user_attributes.user_id AND u.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON warehouses;
CREATE POLICY org_isolation ON warehouses
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM branches b WHERE b.id = warehouses.branch_id AND b.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM branches b WHERE b.id = warehouses.branch_id AND b.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON products;
CREATE POLICY org_isolation ON products
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON product_skus;
CREATE POLICY org_isolation ON product_skus
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM products p WHERE p.id = product_skus.product_id AND p.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM products p WHERE p.id = product_skus.product_id AND p.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON stock_balances;
CREATE POLICY org_isolation ON stock_balances
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1
      FROM warehouses w
      JOIN branches b ON b.id = w.branch_id
      WHERE w.id = stock_balances.warehouse_id AND b.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1
      FROM warehouses w
      JOIN branches b ON b.id = w.branch_id
      WHERE w.id = stock_balances.warehouse_id AND b.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON inventory_movements;
CREATE POLICY org_isolation ON inventory_movements
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON stock_reservations;
CREATE POLICY org_isolation ON stock_reservations
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1
      FROM stock_balances sb
      JOIN warehouses w ON w.id = sb.warehouse_id
      JOIN branches b ON b.id = w.branch_id
      WHERE sb.id = stock_reservations.stock_balance_id AND b.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1
      FROM stock_balances sb
      JOIN warehouses w ON w.id = sb.warehouse_id
      JOIN branches b ON b.id = w.branch_id
      WHERE sb.id = stock_reservations.stock_balance_id AND b.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON employees;
CREATE POLICY org_isolation ON employees
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON employment_contracts;
CREATE POLICY org_isolation ON employment_contracts
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM employees e WHERE e.id = employment_contracts.employee_id AND e.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM employees e WHERE e.id = employment_contracts.employee_id AND e.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON payroll_periods;
CREATE POLICY org_isolation ON payroll_periods
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON payroll_concepts;
CREATE POLICY org_isolation ON payroll_concepts
  FOR ALL
  USING (app_org_matches(org_id))
  WITH CHECK (app_org_matches(org_id));

DROP POLICY IF EXISTS org_isolation ON payroll_runs;
CREATE POLICY org_isolation ON payroll_runs
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM branches b WHERE b.id = payroll_runs.branch_id AND b.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1 FROM branches b WHERE b.id = payroll_runs.branch_id AND b.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON payroll_lines;
CREATE POLICY org_isolation ON payroll_lines
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1
      FROM payroll_runs r
      JOIN branches b ON b.id = r.branch_id
      WHERE r.id = payroll_lines.run_id AND b.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1
      FROM payroll_runs r
      JOIN branches b ON b.id = r.branch_id
      WHERE r.id = payroll_lines.run_id AND b.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON payroll_approvals;
CREATE POLICY org_isolation ON payroll_approvals
  FOR ALL
  USING (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1
      FROM payroll_runs r
      JOIN branches b ON b.id = r.branch_id
      WHERE r.id = payroll_approvals.run_id AND b.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass()
    OR EXISTS (
      SELECT 1
      FROM payroll_runs r
      JOIN branches b ON b.id = r.branch_id
      WHERE r.id = payroll_approvals.run_id AND b.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON audit_log;
CREATE POLICY org_isolation ON audit_log
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

-- Enrichment lookups need bypass or org context; grant execute on helpers
GRANT EXECUTE ON FUNCTION app_rls_bypass() TO PUBLIC;
GRANT EXECUTE ON FUNCTION app_org_id() TO PUBLIC;
GRANT EXECUTE ON FUNCTION app_org_matches(uuid) TO PUBLIC;
