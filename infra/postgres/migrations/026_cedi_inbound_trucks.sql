-- 026_cedi_inbound_trucks.sql
-- Robust CEDI inbound: supplier truck → tarimas (pallets) → cajas (boxes) → stock.

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS inbound_shipments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  warehouse_id UUID NOT NULL REFERENCES warehouses(id),
  receipt_id UUID REFERENCES inbound_receipts(id),
  supplier_name TEXT NOT NULL DEFAULT '',
  invoice_number TEXT NOT NULL DEFAULT '',
  invoice_date DATE,
  carrier_name TEXT NOT NULL DEFAULT '',
  vehicle_ref TEXT NOT NULL DEFAULT '',
  driver_name TEXT NOT NULL DEFAULT '',
  dock_door TEXT NOT NULL DEFAULT '',
  expected_pallets INT NOT NULL DEFAULT 0,
  notes TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'RECEIVING'
    CHECK (status IN ('SCHEDULED', 'ARRIVED', 'RECEIVING', 'POSTED', 'CANCELLED')),
  arrived_at TIMESTAMPTZ,
  posted_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id),
  operator_label TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS inbound_shipments_branch_created_idx
  ON inbound_shipments (branch_id, created_at DESC);

CREATE TABLE IF NOT EXISTS inbound_pallets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  shipment_id UUID NOT NULL REFERENCES inbound_shipments(id) ON DELETE CASCADE,
  pallet_no INT NOT NULL,
  pallet_code TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'OPEN'
    CHECK (status IN ('OPEN', 'COUNTED', 'POSTED')),
  notes TEXT NOT NULL DEFAULT '',
  UNIQUE (shipment_id, pallet_no),
  UNIQUE (shipment_id, pallet_code)
);

CREATE INDEX IF NOT EXISTS inbound_pallets_shipment_idx
  ON inbound_pallets (shipment_id, pallet_no);

CREATE TABLE IF NOT EXISTS inbound_boxes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  pallet_id UUID NOT NULL REFERENCES inbound_pallets(id) ON DELETE CASCADE,
  box_no INT NOT NULL,
  box_code TEXT NOT NULL DEFAULT '',
  sku_id UUID NOT NULL REFERENCES product_skus(id),
  sku_code TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  boxes_count INT NOT NULL DEFAULT 1 CHECK (boxes_count > 0),
  units_per_box NUMERIC(18,4) NOT NULL DEFAULT 1 CHECK (units_per_box > 0),
  quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0), -- boxes_count * units_per_box
  unit_cost NUMERIC(18,4),
  UNIQUE (pallet_id, box_no)
);

CREATE INDEX IF NOT EXISTS inbound_boxes_pallet_idx ON inbound_boxes (pallet_id, box_no);
CREATE INDEX IF NOT EXISTS inbound_boxes_sku_idx ON inbound_boxes (sku_id);

ALTER TABLE inbound_receipts
  ADD COLUMN IF NOT EXISTS shipment_id UUID REFERENCES inbound_shipments(id);

-- RLS
ALTER TABLE inbound_shipments ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbound_shipments FORCE ROW LEVEL SECURITY;
ALTER TABLE inbound_pallets ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbound_pallets FORCE ROW LEVEL SECURITY;
ALTER TABLE inbound_boxes ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbound_boxes FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON inbound_shipments;
CREATE POLICY org_isolation ON inbound_shipments
  USING (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''))
  WITH CHECK (current_setting('app.rls_bypass', true) = 'on' OR org_id::text = NULLIF(current_setting('app.org_id', true), ''));

DROP POLICY IF EXISTS org_isolation ON inbound_pallets;
CREATE POLICY org_isolation ON inbound_pallets
  USING (
    current_setting('app.rls_bypass', true) = 'on'
    OR EXISTS (
      SELECT 1 FROM inbound_shipments s
      WHERE s.id = inbound_pallets.shipment_id
        AND s.org_id::text = NULLIF(current_setting('app.org_id', true), '')
    )
  );

DROP POLICY IF EXISTS org_isolation ON inbound_boxes;
CREATE POLICY org_isolation ON inbound_boxes
  USING (
    current_setting('app.rls_bypass', true) = 'on'
    OR EXISTS (
      SELECT 1 FROM inbound_pallets p
      JOIN inbound_shipments s ON s.id = p.shipment_id
      WHERE p.id = inbound_boxes.pallet_id
        AND s.org_id::text = NULLIF(current_setting('app.org_id', true), '')
    )
  );

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.shipment.read', 'inventory', 'read', 'shipment'),
  ('inventory.shipment.create', 'inventory', 'create', 'shipment'),
  ('inventory.shipment.post', 'inventory', 'post', 'shipment')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'inventory.shipment.read', 'inventory.shipment.create', 'inventory.shipment.post',
  'inventory.receipt.read', 'inventory.receipt.create', 'inventory.receipt.post'
)
WHERE r.code IN (
  'inventory_clerk', 'warehouse_manager', 'regional_manager',
  'store_owner', 'platform_admin'
)
ON CONFLICT DO NOTHING;
