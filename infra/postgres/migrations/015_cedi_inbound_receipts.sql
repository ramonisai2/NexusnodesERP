-- 015_cedi_inbound_receipts.sql
-- Warehouse kinds (CEDI / store / arrival) + inbound receipts with invoice capture
-- and reception labeling hooks.

SELECT set_config('app.rls_bypass', 'on', false);

-- Classify warehouses: CEDI (distribution center), STORE (shop floor stock),
-- ARRIVAL (small-shop single receiving bay / "almacén de llegada").
ALTER TABLE warehouses
  ADD COLUMN IF NOT EXISTS warehouse_kind TEXT NOT NULL DEFAULT 'STORE'
    CHECK (warehouse_kind IN ('STORE', 'CEDI', 'ARRIVAL'));

COMMENT ON COLUMN warehouses.warehouse_kind IS
  'STORE=tienda, CEDI=centro de distribución, ARRIVAL=almacén de llegada (tienda pequeña)';

-- Demo enterprise: mark existing warehouses as STORE; add a CEDI branch+warehouse.
UPDATE warehouses SET warehouse_kind = 'STORE'
WHERE warehouse_kind = 'STORE' OR warehouse_kind IS NULL;

INSERT INTO branches (id, org_id, code, name, region) VALUES
  ('22222222-2222-2222-2222-222222222299', '11111111-1111-1111-1111-111111111111', 'br_cedi', 'CEDI Centro', 'CENTRO')
ON CONFLICT (org_id, code) DO NOTHING;

INSERT INTO warehouses (id, branch_id, code, name, warehouse_kind) VALUES
  (
    '33333333-3333-3333-3333-333333333299',
    '22222222-2222-2222-2222-222222222299',
    'cedi_centro',
    'CEDI Centro — recepción',
    'CEDI'
  )
ON CONFLICT (branch_id, code) DO UPDATE
SET warehouse_kind = EXCLUDED.warehouse_kind,
    name = EXCLUDED.name;

-- Small shops created by setup use code 'principal' → treat as ARRIVAL bay.
UPDATE warehouses w
SET warehouse_kind = 'ARRIVAL'
FROM branches b
WHERE w.branch_id = b.id
  AND w.code = 'principal'
  AND w.warehouse_kind = 'STORE';

CREATE TABLE IF NOT EXISTS inbound_receipts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  warehouse_id UUID NOT NULL REFERENCES warehouses(id),
  supplier_name TEXT NOT NULL DEFAULT '',
  invoice_number TEXT NOT NULL DEFAULT '',
  invoice_date DATE,
  notes TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'DRAFT'
    CHECK (status IN ('DRAFT', 'POSTED', 'VOID')),
  print_labels BOOLEAN NOT NULL DEFAULT TRUE,
  posted_at TIMESTAMPTZ,
  posted_by UUID REFERENCES users(id),
  created_by TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS inbound_receipts_branch_created_idx
  ON inbound_receipts (branch_id, created_at DESC);

CREATE INDEX IF NOT EXISTS inbound_receipts_invoice_idx
  ON inbound_receipts (org_id, invoice_number)
  WHERE invoice_number <> '';

CREATE TABLE IF NOT EXISTS inbound_receipt_lines (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  receipt_id UUID NOT NULL REFERENCES inbound_receipts(id) ON DELETE CASCADE,
  sku_id UUID NOT NULL REFERENCES product_skus(id),
  quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
  unit_cost NUMERIC(18,4),
  label_description TEXT NOT NULL DEFAULT '',
  label_price NUMERIC(18,2),
  movement_id UUID REFERENCES inventory_movements(id),
  sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS inbound_receipt_lines_receipt_idx
  ON inbound_receipt_lines (receipt_id, sort_order);

ALTER TABLE inventory_movements
  ADD COLUMN IF NOT EXISTS source_receipt_id UUID REFERENCES inbound_receipts(id);

ALTER TABLE inbound_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbound_receipts FORCE ROW LEVEL SECURITY;
ALTER TABLE inbound_receipt_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbound_receipt_lines FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON inbound_receipts;
CREATE POLICY org_isolation ON inbound_receipts
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

DROP POLICY IF EXISTS org_isolation ON inbound_receipt_lines;
CREATE POLICY org_isolation ON inbound_receipt_lines
  FOR ALL
  USING (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM inbound_receipts r
      WHERE r.id = inbound_receipt_lines.receipt_id AND r.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM inbound_receipts r
      WHERE r.id = inbound_receipt_lines.receipt_id AND r.org_id::text = app_org_id()
    )
  );

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.receipt.read', 'inventory', 'read', 'receipt'),
  ('inventory.receipt.create', 'inventory', 'create', 'receipt'),
  ('inventory.receipt.post', 'inventory', 'post', 'receipt'),
  ('inventory.warehouse.read', 'inventory', 'read', 'warehouse')
ON CONFLICT (code) DO NOTHING;

-- Clerks, store owners, warehouse/regional managers can receive.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN ('inventory_clerk', 'store_owner', 'warehouse_manager', 'regional_manager', 'platform_admin')
  AND p.code IN ('inventory.receipt.read', 'inventory.receipt.create', 'inventory.receipt.post', 'inventory.warehouse.read')
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inbound_receipts TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inbound_receipt_lines TO nexus;
