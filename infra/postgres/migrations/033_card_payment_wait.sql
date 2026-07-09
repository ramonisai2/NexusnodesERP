-- 033_card_payment_wait.sql
-- Queue for bank-card payments waiting on terminal authorization.

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS pos_card_payment_waits (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  branch_id UUID NOT NULL REFERENCES branches(id),
  status TEXT NOT NULL DEFAULT 'WAITING'
    CHECK (status IN ('WAITING', 'APPROVED', 'DECLINED', 'CANCELLED', 'EXPIRED')),
  amount NUMERIC(18,2) NOT NULL CHECK (amount > 0),
  currency TEXT NOT NULL DEFAULT 'MXN',
  customer_name TEXT NOT NULL DEFAULT '',
  lines JSONB NOT NULL DEFAULT '[]'::jsonb,
  warehouse_code TEXT NOT NULL DEFAULT '',
  request_invoice BOOLEAN NOT NULL DEFAULT FALSE,
  invoice_rfc TEXT NOT NULL DEFAULT '',
  invoice_name TEXT NOT NULL DEFAULT '',
  invoice_email TEXT NOT NULL DEFAULT '',
  invoice_uso_cfdi TEXT NOT NULL DEFAULT 'G01',
  notes TEXT NOT NULL DEFAULT '',
  terminal_ref TEXT NOT NULL DEFAULT '',
  auth_code TEXT NOT NULL DEFAULT '',
  decline_reason TEXT NOT NULL DEFAULT '',
  sale_id UUID REFERENCES pos_sales(id),
  created_by TEXT NOT NULL DEFAULT '',
  operator_label TEXT NOT NULL DEFAULT '',
  station_id TEXT NOT NULL DEFAULT '',
  confirmed_by TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  UNIQUE (org_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS pos_card_waits_branch_status_idx
  ON pos_card_payment_waits (branch_id, status, created_at DESC);

ALTER TABLE pos_card_payment_waits ENABLE ROW LEVEL SECURITY;
ALTER TABLE pos_card_payment_waits FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON pos_card_payment_waits;
CREATE POLICY org_isolation ON pos_card_payment_waits
  USING (org_id::text = app_org_id() OR current_setting('app.rls_bypass', true) = 'on')
  WITH CHECK (org_id::text = app_org_id() OR current_setting('app.rls_bypass', true) = 'on');

INSERT INTO permissions (code, module, action, resource) VALUES
  ('pos.card.wait.read', 'pos', 'read', 'card_wait'),
  ('pos.card.wait.create', 'pos', 'create', 'card_wait'),
  ('pos.card.wait.confirm', 'pos', 'confirm', 'card_wait'),
  ('pos.card.wait.cancel', 'pos', 'cancel', 'card_wait')
ON CONFLICT (code) DO NOTHING;

-- Grant to POS-capable roles
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'pos.card.wait.read', 'pos.card.wait.create', 'pos.card.wait.confirm', 'pos.card.wait.cancel'
)
WHERE r.code IN (
  'store_owner', 'store_admin', 'store_coordinator', 'cashier',
  'platform_admin', 'regional_manager', 'area_manager', 'warehouse_manager'
)
ON CONFLICT DO NOTHING;

-- Sales associates can create/read waits but not confirm (SoD-ish optional)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('pos.card.wait.read', 'pos.card.wait.create')
WHERE r.code IN ('sales_associate')
ON CONFLICT DO NOTHING;

-- Demo explorer / webmaster visibility
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'pos.card.wait.read', 'pos.card.wait.create', 'pos.card.wait.confirm', 'pos.card.wait.cancel'
)
WHERE r.code IN ('webmaster')
ON CONFLICT DO NOTHING;
