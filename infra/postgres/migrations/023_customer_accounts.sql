-- 023_customer_accounts.sql
-- Cuentas de cliente (auto-registro) y tarjetas con chip o código de barras.

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS customers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  email TEXT NOT NULL,
  phone TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE', 'SUSPENDED', 'CLOSED')),
  preferred_branch_id UUID REFERENCES branches(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, email)
);

CREATE INDEX IF NOT EXISTS customers_org_status_idx
  ON customers (org_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS customers_org_phone_idx
  ON customers (org_id, phone)
  WHERE phone <> '';

CREATE TABLE IF NOT EXISTS customer_cards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  card_kind TEXT NOT NULL
    CHECK (card_kind IN ('BARCODE', 'CHIP')),
  card_code TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE', 'BLOCKED', 'LOST', 'EXPIRED')),
  issued_by TEXT NOT NULL DEFAULT '',
  blocked_reason TEXT NOT NULL DEFAULT '',
  blocked_at TIMESTAMPTZ,
  last_seen_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, card_code)
);

CREATE INDEX IF NOT EXISTS customer_cards_customer_idx
  ON customer_cards (customer_id, status);

CREATE INDEX IF NOT EXISTS customer_cards_org_kind_idx
  ON customer_cards (org_id, card_kind, status);

COMMENT ON TABLE customers IS
  'Clientes finales con cuenta propia (registro en tienda en línea).';
COMMENT ON TABLE customer_cards IS
  'Tarjetas de cliente: BARCODE (código de barras) o CHIP (UID de chip/NFC).';
COMMENT ON COLUMN customer_cards.card_code IS
  'Código escaneable: barcode numérico/alfanumérico o UID del chip.';

ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers FORCE ROW LEVEL SECURITY;
ALTER TABLE customer_cards ENABLE ROW LEVEL SECURITY;
ALTER TABLE customer_cards FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON customers;
CREATE POLICY org_isolation ON customers
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

DROP POLICY IF EXISTS org_isolation ON customer_cards;
CREATE POLICY org_isolation ON customer_cards
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

INSERT INTO permissions (code, module, action, resource) VALUES
  ('customer.read', 'customer', 'read', 'customer'),
  ('customer.manage', 'customer', 'manage', 'customer'),
  ('customer.card.read', 'customer', 'read', 'customer_card'),
  ('customer.card.manage', 'customer', 'manage', 'customer_card'),
  ('customer.self.read', 'customer', 'self_read', 'customer'),
  ('customer.card.self', 'customer', 'self_manage', 'customer_card')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN (
  'inventory_clerk', 'warehouse_manager', 'regional_manager',
  'store_owner', 'platform_admin'
)
  AND p.code IN (
    'customer.read', 'customer.card.read',
    'customer.card.manage'
  )
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN (
  'warehouse_manager', 'regional_manager', 'store_owner', 'platform_admin'
)
  AND p.code IN ('customer.manage')
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE customers TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE customer_cards TO nexus;
