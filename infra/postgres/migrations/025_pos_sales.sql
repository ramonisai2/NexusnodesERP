-- 025_pos_sales.sql
-- Caja de cobro (POS): tickets con IVA MX, pagos, opción de facturar (CFDI pendiente de PAC).

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS branch_fiscal_settings (
  branch_id UUID PRIMARY KEY REFERENCES branches(id) ON DELETE CASCADE,
  org_id UUID NOT NULL REFERENCES organizations(id),
  legal_name TEXT NOT NULL DEFAULT '',
  trade_name TEXT NOT NULL DEFAULT '',
  rfc TEXT NOT NULL DEFAULT '',
  tax_regime TEXT NOT NULL DEFAULT '601', -- Régimen General de Ley Personas Morales (default demo)
  postal_code TEXT NOT NULL DEFAULT '',
  prices_include_tax BOOLEAN NOT NULL DEFAULT TRUE, -- típico abarrotes MX: precio con IVA
  default_tax_rate NUMERIC(8,4) NOT NULL DEFAULT 0.1600, -- IVA 16%
  ticket_series TEXT NOT NULL DEFAULT 'T',
  next_ticket_folio BIGINT NOT NULL DEFAULT 1,
  invoice_series TEXT NOT NULL DEFAULT 'F',
  next_invoice_folio BIGINT NOT NULL DEFAULT 1,
  receipt_footer TEXT NOT NULL DEFAULT 'Gracias por su compra. Conserve este ticket.',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS customer_fiscal_profiles (
  customer_id UUID PRIMARY KEY REFERENCES customers(id) ON DELETE CASCADE,
  org_id UUID NOT NULL REFERENCES organizations(id),
  rfc TEXT NOT NULL DEFAULT '',
  legal_name TEXT NOT NULL DEFAULT '',
  tax_regime TEXT NOT NULL DEFAULT '616', -- Sin obligaciones fiscales (público en general / PF)
  postal_code TEXT NOT NULL DEFAULT '',
  uso_cfdi TEXT NOT NULL DEFAULT 'G03', -- Gastos en general
  email_cfdi TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pos_sales (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  warehouse_id UUID REFERENCES warehouses(id),
  ticket_number TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'COMPLETED'
    CHECK (status IN ('DRAFT', 'COMPLETED', 'VOID')),
  currency TEXT NOT NULL DEFAULT 'MXN',
  prices_include_tax BOOLEAN NOT NULL DEFAULT TRUE,
  tax_rate NUMERIC(8,4) NOT NULL DEFAULT 0.1600,
  subtotal NUMERIC(18,2) NOT NULL DEFAULT 0,      -- base gravable (sin IVA)
  tax_total NUMERIC(18,2) NOT NULL DEFAULT 0,     -- IVA
  discount_total NUMERIC(18,2) NOT NULL DEFAULT 0,
  grand_total NUMERIC(18,2) NOT NULL DEFAULT 0,    -- total a pagar (con IVA si prices_include_tax)
  customer_id UUID REFERENCES customers(id),
  customer_name TEXT NOT NULL DEFAULT '',
  cashier_sub TEXT NOT NULL DEFAULT '',
  operator_label TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  station_id TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  request_invoice BOOLEAN NOT NULL DEFAULT FALSE,
  idempotency_key TEXT NOT NULL,
  completed_at TIMESTAMPTZ,
  voided_at TIMESTAMPTZ,
  void_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, idempotency_key),
  UNIQUE (org_id, ticket_number)
);

CREATE INDEX IF NOT EXISTS pos_sales_branch_completed_idx
  ON pos_sales (branch_id, completed_at DESC)
  WHERE status = 'COMPLETED';

CREATE TABLE IF NOT EXISTS pos_sale_lines (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_id UUID NOT NULL REFERENCES pos_sales(id) ON DELETE CASCADE,
  line_no INT NOT NULL,
  sku_id UUID NOT NULL REFERENCES product_skus(id),
  sku_code TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
  unit_price NUMERIC(18,2) NOT NULL,          -- precio unitario cobrado (con IVA si include)
  line_total NUMERIC(18,2) NOT NULL,          -- quantity * unit_price
  tax_rate NUMERIC(8,4) NOT NULL DEFAULT 0.1600,
  tax_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
  base_amount NUMERIC(18,2) NOT NULL DEFAULT 0, -- sin IVA
  movement_id UUID REFERENCES inventory_movements(id),
  UNIQUE (sale_id, line_no)
);

CREATE TABLE IF NOT EXISTS pos_payments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_id UUID NOT NULL REFERENCES pos_sales(id) ON DELETE CASCADE,
  method TEXT NOT NULL CHECK (method IN ('CASH', 'CARD', 'TRANSFER', 'OTHER')),
  amount NUMERIC(18,2) NOT NULL CHECK (amount >= 0),
  received_amount NUMERIC(18,2),
  change_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
  reference TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pos_payments_sale_idx ON pos_payments (sale_id);

CREATE TABLE IF NOT EXISTS fiscal_invoices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  sale_id UUID NOT NULL REFERENCES pos_sales(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'REQUESTED'
    CHECK (status IN ('REQUESTED', 'STAMPED', 'CANCELLED', 'ERROR')),
  series TEXT NOT NULL DEFAULT 'F',
  folio BIGINT,
  uuid TEXT, -- CFDI UUID when stamped by PAC
  rfc_receiver TEXT NOT NULL DEFAULT '',
  legal_name_receiver TEXT NOT NULL DEFAULT '',
  tax_regime_receiver TEXT NOT NULL DEFAULT '',
  postal_code_receiver TEXT NOT NULL DEFAULT '',
  uso_cfdi TEXT NOT NULL DEFAULT 'G03',
  email_cfdi TEXT NOT NULL DEFAULT '',
  subtotal NUMERIC(18,2) NOT NULL DEFAULT 0,
  tax_total NUMERIC(18,2) NOT NULL DEFAULT 0,
  grand_total NUMERIC(18,2) NOT NULL DEFAULT 0,
  notes TEXT NOT NULL DEFAULT 'Pendiente de timbrado CFDI (PAC).',
  requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  stamped_at TIMESTAMPTZ,
  UNIQUE (sale_id)
);

CREATE INDEX IF NOT EXISTS fiscal_invoices_org_status_idx
  ON fiscal_invoices (org_id, status, requested_at DESC);

ALTER TABLE inventory_movements
  ADD COLUMN IF NOT EXISTS source_sale_id UUID REFERENCES pos_sales(id);

-- RLS
ALTER TABLE branch_fiscal_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE branch_fiscal_settings FORCE ROW LEVEL SECURITY;
ALTER TABLE customer_fiscal_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE customer_fiscal_profiles FORCE ROW LEVEL SECURITY;
ALTER TABLE pos_sales ENABLE ROW LEVEL SECURITY;
ALTER TABLE pos_sales FORCE ROW LEVEL SECURITY;
ALTER TABLE pos_sale_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE pos_sale_lines FORCE ROW LEVEL SECURITY;
ALTER TABLE pos_payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE pos_payments FORCE ROW LEVEL SECURITY;
ALTER TABLE fiscal_invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE fiscal_invoices FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON branch_fiscal_settings;
CREATE POLICY org_isolation ON branch_fiscal_settings
  USING (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''))
  WITH CHECK (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''));

DROP POLICY IF EXISTS org_isolation ON customer_fiscal_profiles;
CREATE POLICY org_isolation ON customer_fiscal_profiles
  USING (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''))
  WITH CHECK (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''));

DROP POLICY IF EXISTS org_isolation ON pos_sales;
CREATE POLICY org_isolation ON pos_sales
  USING (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''))
  WITH CHECK (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''));

DROP POLICY IF EXISTS org_isolation ON pos_sale_lines;
CREATE POLICY org_isolation ON pos_sale_lines
  USING (
    current_setting('app.rls_bypass', true) = 'on'
    OR EXISTS (
      SELECT 1 FROM pos_sales s
      WHERE s.id = pos_sale_lines.sale_id
        AND s.org_id::text = NULLIF(current_setting('app.org_id', true), '')
    )
  );

DROP POLICY IF EXISTS org_isolation ON pos_payments;
CREATE POLICY org_isolation ON pos_payments
  USING (
    current_setting('app.rls_bypass', true) = 'on'
    OR EXISTS (
      SELECT 1 FROM pos_sales s
      WHERE s.id = pos_payments.sale_id
        AND s.org_id::text = NULLIF(current_setting('app.org_id', true), '')
    )
  );

DROP POLICY IF EXISTS org_isolation ON fiscal_invoices;
CREATE POLICY org_isolation ON fiscal_invoices
  USING (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''))
  WITH CHECK (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''));

INSERT INTO permissions (code, module, action, resource) VALUES
  ('pos.sale.read', 'pos', 'read', 'sale'),
  ('pos.sale.create', 'pos', 'create', 'sale'),
  ('pos.sale.void', 'pos', 'void', 'sale'),
  ('pos.invoice.request', 'pos', 'request', 'invoice'),
  ('pos.invoice.read', 'pos', 'read', 'invoice'),
  ('pos.settings.manage', 'pos', 'manage', 'settings')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'pos.sale.read', 'pos.sale.create', 'pos.invoice.request', 'pos.invoice.read'
)
WHERE r.code IN (
  'inventory_clerk', 'warehouse_manager', 'regional_manager',
  'store_owner', 'platform_admin'
)
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('pos.sale.void', 'pos.settings.manage')
WHERE r.code IN ('warehouse_manager', 'regional_manager', 'store_owner', 'platform_admin')
ON CONFLICT DO NOTHING;

-- Demo fiscal settings for known branches
INSERT INTO branch_fiscal_settings (
  branch_id, org_id, legal_name, trade_name, rfc, tax_regime, postal_code,
  prices_include_tax, default_tax_rate, receipt_footer
)
SELECT b.id, b.org_id,
  'Comercial Demo SA de CV',
  b.name,
  'CDE010101AAA',
  '601',
  '64000',
  TRUE,
  0.1600,
  'Gracias por su compra. Ticket con IVA incluido. Solicite factura con su RFC.'
FROM branches b
ON CONFLICT (branch_id) DO NOTHING;
