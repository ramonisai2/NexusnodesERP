-- 020_inventory_transfers.sql
-- Transferencias atómicas entre almacenes/sucursales: DRAFT → IN_TRANSIT → RECEIVED.

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS inventory_transfers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  transfer_number TEXT NOT NULL,
  from_branch_id UUID NOT NULL REFERENCES branches(id),
  to_branch_id UUID NOT NULL REFERENCES branches(id),
  from_warehouse_id UUID NOT NULL REFERENCES warehouses(id),
  to_warehouse_id UUID NOT NULL REFERENCES warehouses(id),
  status TEXT NOT NULL DEFAULT 'DRAFT'
    CHECK (status IN ('DRAFT', 'IN_TRANSIT', 'RECEIVED', 'CANCELLED')),
  notes TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  operator_label TEXT NOT NULL DEFAULT '',
  session_id UUID,
  shipped_by TEXT NOT NULL DEFAULT '',
  shipped_at TIMESTAMPTZ,
  received_by TEXT NOT NULL DEFAULT '',
  received_at TIMESTAMPTZ,
  cancelled_by TEXT NOT NULL DEFAULT '',
  cancelled_at TIMESTAMPTZ,
  cancel_reason TEXT NOT NULL DEFAULT '',
  transport_sheet_id UUID,
  idempotency_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, transfer_number),
  UNIQUE (org_id, idempotency_key),
  CHECK (from_branch_id <> to_branch_id OR from_warehouse_id <> to_warehouse_id)
);

CREATE INDEX IF NOT EXISTS inventory_transfers_org_status_idx
  ON inventory_transfers (org_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS inventory_transfers_from_idx
  ON inventory_transfers (org_id, from_branch_id, created_at DESC);

CREATE INDEX IF NOT EXISTS inventory_transfers_to_idx
  ON inventory_transfers (org_id, to_branch_id, created_at DESC);

CREATE TABLE IF NOT EXISTS inventory_transfer_lines (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  transfer_id UUID NOT NULL REFERENCES inventory_transfers(id) ON DELETE CASCADE,
  sku_id UUID NOT NULL REFERENCES product_skus(id),
  quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
  sort_order INT NOT NULL DEFAULT 0,
  out_movement_id UUID REFERENCES inventory_movements(id),
  in_movement_id UUID REFERENCES inventory_movements(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS inventory_transfer_lines_transfer_idx
  ON inventory_transfer_lines (transfer_id, sort_order);

CREATE TABLE IF NOT EXISTS inventory_transfer_slip_links (
  transfer_id UUID NOT NULL REFERENCES inventory_transfers(id) ON DELETE CASCADE,
  shipping_slip_id UUID NOT NULL REFERENCES shipping_slips(id) ON DELETE CASCADE,
  PRIMARY KEY (transfer_id, shipping_slip_id)
);

ALTER TABLE inventory_movements
  ADD COLUMN IF NOT EXISTS source_transfer_id UUID REFERENCES inventory_transfers(id);

ALTER TABLE inventory_movements
  ADD COLUMN IF NOT EXISTS transfer_line_id UUID REFERENCES inventory_transfer_lines(id);

CREATE INDEX IF NOT EXISTS inventory_movements_transfer_idx
  ON inventory_movements (source_transfer_id)
  WHERE source_transfer_id IS NOT NULL;

COMMENT ON TABLE inventory_transfers IS
  'Traslado de stock entre almacenes: sale (TRANSFER_OUT) al embarcar y entra (TRANSFER_IN) al recibir.';
COMMENT ON COLUMN inventory_transfers.status IS
  'DRAFT (sin stock) → IN_TRANSIT (salida) → RECEIVED (entrada) | CANCELLED';

ALTER TABLE inventory_transfers ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_transfers FORCE ROW LEVEL SECURITY;
ALTER TABLE inventory_transfer_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_transfer_lines FORCE ROW LEVEL SECURITY;
ALTER TABLE inventory_transfer_slip_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_transfer_slip_links FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON inventory_transfers;
CREATE POLICY org_isolation ON inventory_transfers
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

DROP POLICY IF EXISTS org_isolation ON inventory_transfer_lines;
CREATE POLICY org_isolation ON inventory_transfer_lines
  FOR ALL
  USING (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM inventory_transfers t
      WHERE t.id = inventory_transfer_lines.transfer_id
        AND t.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM inventory_transfers t
      WHERE t.id = inventory_transfer_lines.transfer_id
        AND t.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON inventory_transfer_slip_links;
CREATE POLICY org_isolation ON inventory_transfer_slip_links
  FOR ALL
  USING (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM inventory_transfers t
      WHERE t.id = inventory_transfer_slip_links.transfer_id
        AND t.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM inventory_transfers t
      WHERE t.id = inventory_transfer_slip_links.transfer_id
        AND t.org_id::text = app_org_id()
    )
  );

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.transfer.read', 'inventory', 'read', 'transfer'),
  ('inventory.transfer.create', 'inventory', 'create', 'transfer'),
  ('inventory.transfer.ship', 'inventory', 'ship', 'transfer'),
  ('inventory.transfer.receive', 'inventory', 'receive', 'transfer'),
  ('inventory.transfer.cancel', 'inventory', 'cancel', 'transfer')
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
    'inventory.transfer.read', 'inventory.transfer.create',
    'inventory.transfer.ship', 'inventory.transfer.receive'
  )
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN (
  'warehouse_manager', 'regional_manager', 'store_owner', 'platform_admin'
)
  AND p.code = 'inventory.transfer.cancel'
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inventory_transfers TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inventory_transfer_lines TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inventory_transfer_slip_links TO nexus;
