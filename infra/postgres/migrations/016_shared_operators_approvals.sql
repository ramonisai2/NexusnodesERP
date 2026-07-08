-- 016_shared_operators_approvals.sql
-- Shared account → many concurrent operator stations; critical actions need boss approval.

SELECT set_config('app.rls_bypass', 'on', false);

-- Operator stations on top of the existing sessions table.
ALTER TABLE sessions
  ADD COLUMN IF NOT EXISTS operator_label TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS station_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS jti TEXT,
  ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);

CREATE INDEX IF NOT EXISTS sessions_user_active_idx
  ON sessions (user_id)
  WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS sessions_jti_idx
  ON sessions (jti)
  WHERE jti IS NOT NULL AND revoked_at IS NULL;

COMMENT ON COLUMN sessions.operator_label IS
  'Human at the shared username (e.g. Ana en caja 2). Many rows per user_id = concurrent activity.';
COMMENT ON COLUMN sessions.station_id IS
  'Optional terminal / POS / device code for concurrent stations.';

-- Attribute inventory facts to the operator seat, not only the shared account.
ALTER TABLE inventory_movements
  ADD COLUMN IF NOT EXISTS operator_label TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS session_id UUID REFERENCES sessions(id);

ALTER TABLE inbound_receipts
  ADD COLUMN IF NOT EXISTS operator_label TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS session_id UUID REFERENCES sessions(id);

-- Generic maker-checker queue (voids, high-value receipts, …).
CREATE TABLE IF NOT EXISTS approval_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  action_code TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  branch_id TEXT NOT NULL DEFAULT '',
  warehouse_id TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  summary TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'PENDING'
    CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'EXPIRED', 'CANCELLED')),
  requested_by_sub TEXT NOT NULL,
  requested_by_operator TEXT NOT NULL DEFAULT '',
  requested_by_session UUID REFERENCES sessions(id),
  requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  decided_by_sub TEXT,
  decided_by_operator TEXT NOT NULL DEFAULT '',
  decided_at TIMESTAMPTZ,
  decision_reason TEXT NOT NULL DEFAULT '',
  result_ref TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS approval_requests_one_pending_uidx
  ON approval_requests (org_id, resource_type, resource_id, action_code)
  WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS approval_requests_pending_idx
  ON approval_requests (org_id, status, requested_at DESC);

ALTER TABLE approval_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE approval_requests FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON approval_requests;
CREATE POLICY org_isolation ON approval_requests
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.movement.void.request', 'inventory', 'request', 'movement_void'),
  ('approval.decide', 'approval', 'decide', 'request'),
  ('approval.read', 'approval', 'read', 'request'),
  ('session.operator', 'auth', 'operate', 'session')
ON CONFLICT (code) DO NOTHING;

-- Clerks can REQUEST void; managers decide; everyone with balance can read own requests.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN ('inventory_clerk', 'store_owner')
  AND p.code IN ('inventory.movement.void.request', 'approval.read', 'session.operator')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN ('warehouse_manager', 'regional_manager', 'platform_admin', 'payroll_approver')
  AND p.code IN ('approval.decide', 'approval.read', 'session.operator')
ON CONFLICT DO NOTHING;

-- Analysts (clerks) also get session.operator for shared-desk demos.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN ('inventory_clerk', 'payroll_analyst', 'warehouse_manager', 'regional_manager', 'store_owner', 'platform_admin')
  AND p.code = 'session.operator'
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE approval_requests TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE sessions TO nexus;
