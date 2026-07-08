-- 017_shipping_slips.sql
-- Inter-store papelería: printable identification slips stuck on containers
-- (sobre, caja, caja plástica, bulto, empaque original).

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS shipping_slips (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  slip_number TEXT NOT NULL,
  from_branch_id UUID NOT NULL REFERENCES branches(id),
  to_branch_id UUID NOT NULL REFERENCES branches(id),
  from_warehouse_id UUID REFERENCES warehouses(id),
  to_warehouse_id UUID REFERENCES warehouses(id),
  container_type TEXT NOT NULL
    CHECK (container_type IN (
      'ENVELOPE',      -- sobre
      'BOX',           -- caja
      'PLASTIC_BOX',   -- caja plástica
      'BUNDLE',        -- bulto
      'ORIGINAL_PACK'  -- empaque original
    )),
  description TEXT NOT NULL DEFAULT '',
  contents_summary TEXT NOT NULL DEFAULT '',
  quantity_units INT NOT NULL DEFAULT 1 CHECK (quantity_units > 0),
  status TEXT NOT NULL DEFAULT 'DRAFT'
    CHECK (status IN ('DRAFT', 'PRINTED', 'IN_TRANSIT', 'RECEIVED', 'CANCELLED')),
  printed_at TIMESTAMPTZ,
  shipped_at TIMESTAMPTZ,
  received_at TIMESTAMPTZ,
  created_by TEXT NOT NULL DEFAULT '',
  operator_label TEXT NOT NULL DEFAULT '',
  session_id UUID REFERENCES sessions(id),
  notes TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, idempotency_key),
  UNIQUE (org_id, slip_number)
);

CREATE INDEX IF NOT EXISTS shipping_slips_org_status_idx
  ON shipping_slips (org_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS shipping_slips_from_branch_idx
  ON shipping_slips (from_branch_id, created_at DESC);

CREATE INDEX IF NOT EXISTS shipping_slips_to_branch_idx
  ON shipping_slips (to_branch_id, created_at DESC);

CREATE TABLE IF NOT EXISTS shipping_slip_lines (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slip_id UUID NOT NULL REFERENCES shipping_slips(id) ON DELETE CASCADE,
  sku_id UUID REFERENCES product_skus(id),
  sku_code TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  quantity NUMERIC(18,4) NOT NULL DEFAULT 1 CHECK (quantity > 0),
  sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS shipping_slip_lines_slip_idx
  ON shipping_slip_lines (slip_id, sort_order);

COMMENT ON TABLE shipping_slips IS
  'Papeletas de identificación entre tiendas: se imprimen y pegan en el contenedor.';
COMMENT ON COLUMN shipping_slips.container_type IS
  'ENVELOPE=sobre, BOX=caja, PLASTIC_BOX=caja plástica, BUNDLE=bulto, ORIGINAL_PACK=empaque original';

ALTER TABLE shipping_slips ENABLE ROW LEVEL SECURITY;
ALTER TABLE shipping_slips FORCE ROW LEVEL SECURITY;
ALTER TABLE shipping_slip_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE shipping_slip_lines FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON shipping_slips;
CREATE POLICY org_isolation ON shipping_slips
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

DROP POLICY IF EXISTS org_isolation ON shipping_slip_lines;
CREATE POLICY org_isolation ON shipping_slip_lines
  FOR ALL
  USING (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM shipping_slips s
      WHERE s.id = shipping_slip_lines.slip_id
        AND s.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM shipping_slips s
      WHERE s.id = shipping_slip_lines.slip_id
        AND s.org_id::text = app_org_id()
    )
  );

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.slip.read', 'inventory', 'read', 'shipping_slip'),
  ('inventory.slip.create', 'inventory', 'create', 'shipping_slip'),
  ('inventory.slip.print', 'inventory', 'print', 'shipping_slip')
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
    'inventory.slip.read', 'inventory.slip.create', 'inventory.slip.print'
  )
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE shipping_slips TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE shipping_slip_lines TO nexus;
