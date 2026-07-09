-- 022_parcel_logistics.sql
-- Paquetería robusta: tipos de envío (tienda/CEDI/defectuosos/garantías),
-- ciclos de vida de papeletas y hojas de transporte, casos de garantía/devolución.

SELECT set_config('app.rls_bypass', 'on', false);

-- Branch kinds: store, CEDI, service/repair center.
ALTER TABLE branches
  ADD COLUMN IF NOT EXISTS branch_kind TEXT NOT NULL DEFAULT 'STORE';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'branches_branch_kind_check'
  ) THEN
    ALTER TABLE branches
      ADD CONSTRAINT branches_branch_kind_check
      CHECK (branch_kind IN ('STORE', 'CEDI', 'SERVICE_CENTER'));
  END IF;
END $$;

UPDATE branches SET branch_kind = 'CEDI' WHERE code = 'br_cedi' AND branch_kind = 'STORE';

-- Warehouse kinds for quarantine / defective / repair / returns.
ALTER TABLE warehouses DROP CONSTRAINT IF EXISTS warehouses_warehouse_kind_check;
ALTER TABLE warehouses
  ADD CONSTRAINT warehouses_warehouse_kind_check
  CHECK (warehouse_kind IN (
    'STORE', 'CEDI', 'ARRIVAL', 'DEFECTIVE', 'QUARANTINE', 'REPAIR', 'RETURNS'
  ));

-- Seed defective / returns warehouses for demo CEDI and norte (idempotent).
INSERT INTO warehouses (id, branch_id, code, name, warehouse_kind)
SELECT gen_random_uuid(), b.id, 'defective', 'Defectuosos', 'DEFECTIVE'
FROM branches b WHERE b.code IN ('br_cedi', 'br_norte')
  AND NOT EXISTS (
    SELECT 1 FROM warehouses w WHERE w.branch_id = b.id AND w.code = 'defective'
  );

INSERT INTO warehouses (id, branch_id, code, name, warehouse_kind)
SELECT gen_random_uuid(), b.id, 'returns', 'Devoluciones', 'RETURNS'
FROM branches b WHERE b.code = 'br_cedi'
  AND NOT EXISTS (
    SELECT 1 FROM warehouses w WHERE w.branch_id = b.id AND w.code = 'returns'
  );

INSERT INTO warehouses (id, branch_id, code, name, warehouse_kind)
SELECT gen_random_uuid(), b.id, 'repair', 'Taller / garantías', 'REPAIR'
FROM branches b WHERE b.code = 'br_cedi'
  AND NOT EXISTS (
    SELECT 1 FROM warehouses w WHERE w.branch_id = b.id AND w.code = 'repair'
  );

-- Parcel kinds on slips / transfers / transport sheets.
ALTER TABLE shipping_slips
  ADD COLUMN IF NOT EXISTS parcel_kind TEXT NOT NULL DEFAULT 'TRANSFER';

ALTER TABLE shipping_slips
  ADD COLUMN IF NOT EXISTS tracking_code TEXT NOT NULL DEFAULT '';

ALTER TABLE shipping_slips
  ADD COLUMN IF NOT EXISTS reference_type TEXT NOT NULL DEFAULT '';

ALTER TABLE shipping_slips
  ADD COLUMN IF NOT EXISTS reference_id UUID;

ALTER TABLE shipping_slips
  ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

ALTER TABLE shipping_slips
  ADD COLUMN IF NOT EXISTS cancel_reason TEXT NOT NULL DEFAULT '';

-- Allow same-branch slips when warehouses differ (intra-store quarantine moves).
ALTER TABLE shipping_slips DROP CONSTRAINT IF EXISTS shipping_slips_from_branch_id_to_branch_id_check;
-- No hard CHECK on branches differing; validated in app when warehouses equal.

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'shipping_slips_parcel_kind_check'
  ) THEN
    ALTER TABLE shipping_slips
      ADD CONSTRAINT shipping_slips_parcel_kind_check
      CHECK (parcel_kind IN (
        'TRANSFER', 'CEDI_DISTRIBUTION', 'DEFECTIVE', 'WARRANTY',
        'RETURN_TO_CEDI', 'RETURN_TO_VENDOR', 'REPAIR_OUT', 'REPAIR_IN'
      ));
  END IF;
END $$;

ALTER TABLE inventory_transfers
  ADD COLUMN IF NOT EXISTS parcel_kind TEXT NOT NULL DEFAULT 'TRANSFER';

ALTER TABLE inventory_transfers
  ADD COLUMN IF NOT EXISTS transport_sheet_id UUID REFERENCES transport_sheets(id);

ALTER TABLE inventory_transfers
  ADD COLUMN IF NOT EXISTS warranty_case_id UUID;

ALTER TABLE inventory_transfers
  ADD COLUMN IF NOT EXISTS return_case_id UUID;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'inventory_transfers_parcel_kind_check'
  ) THEN
    ALTER TABLE inventory_transfers
      ADD CONSTRAINT inventory_transfers_parcel_kind_check
      CHECK (parcel_kind IN (
        'TRANSFER', 'CEDI_DISTRIBUTION', 'DEFECTIVE', 'WARRANTY',
        'RETURN_TO_CEDI', 'RETURN_TO_VENDOR', 'REPAIR_OUT', 'REPAIR_IN'
      ));
  END IF;
END $$;

ALTER TABLE transport_sheets
  ADD COLUMN IF NOT EXISTS parcel_kind TEXT NOT NULL DEFAULT 'TRANSFER';

ALTER TABLE transport_sheets
  ADD COLUMN IF NOT EXISTS inventory_transfer_id UUID REFERENCES inventory_transfers(id);

ALTER TABLE transport_sheets
  ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

ALTER TABLE transport_sheets
  ADD COLUMN IF NOT EXISTS cancel_reason TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'transport_sheets_parcel_kind_check'
  ) THEN
    ALTER TABLE transport_sheets
      ADD CONSTRAINT transport_sheets_parcel_kind_check
      CHECK (parcel_kind IN (
        'TRANSFER', 'CEDI_DISTRIBUTION', 'DEFECTIVE', 'WARRANTY',
        'RETURN_TO_CEDI', 'RETURN_TO_VENDOR', 'REPAIR_OUT', 'REPAIR_IN'
      ));
  END IF;
END $$;

-- Warranty / defective cases (customer or internal).
CREATE TABLE IF NOT EXISTS warranty_cases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  case_number TEXT NOT NULL,
  branch_id UUID NOT NULL REFERENCES branches(id),
  destination_branch_id UUID REFERENCES branches(id),
  sku_id UUID REFERENCES product_skus(id),
  sku_code TEXT NOT NULL DEFAULT '',
  serial_number TEXT NOT NULL DEFAULT '',
  customer_ref TEXT NOT NULL DEFAULT '',
  problem_description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'OPEN'
    CHECK (status IN ('OPEN', 'SHIPPED', 'RECEIVED', 'IN_REPAIR', 'CLOSED', 'CANCELLED')),
  parcel_kind TEXT NOT NULL DEFAULT 'WARRANTY',
  shipping_slip_id UUID REFERENCES shipping_slips(id),
  transfer_id UUID REFERENCES inventory_transfers(id),
  created_by TEXT NOT NULL DEFAULT '',
  operator_label TEXT NOT NULL DEFAULT '',
  closed_at TIMESTAMPTZ,
  idempotency_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, case_number),
  UNIQUE (org_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS warranty_cases_org_status_idx
  ON warranty_cases (org_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS return_cases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  case_number TEXT NOT NULL,
  from_branch_id UUID NOT NULL REFERENCES branches(id),
  to_branch_id UUID NOT NULL REFERENCES branches(id),
  reason TEXT NOT NULL DEFAULT 'DEFECTIVE'
    CHECK (reason IN ('DEFECTIVE', 'WARRANTY', 'CUSTOMER_RETURN', 'OVERSTOCK', 'OTHER')),
  notes TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'OPEN'
    CHECK (status IN ('OPEN', 'SHIPPED', 'RECEIVED', 'CLOSED', 'CANCELLED')),
  parcel_kind TEXT NOT NULL DEFAULT 'DEFECTIVE',
  shipping_slip_id UUID REFERENCES shipping_slips(id),
  transfer_id UUID REFERENCES inventory_transfers(id),
  created_by TEXT NOT NULL DEFAULT '',
  operator_label TEXT NOT NULL DEFAULT '',
  closed_at TIMESTAMPTZ,
  idempotency_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, case_number),
  UNIQUE (org_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS return_cases_org_status_idx
  ON return_cases (org_id, status, created_at DESC);

-- Wire FKs from transfers to cases (added after tables exist).
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'inventory_transfers_warranty_case_id_fkey'
  ) THEN
    ALTER TABLE inventory_transfers
      ADD CONSTRAINT inventory_transfers_warranty_case_id_fkey
      FOREIGN KEY (warranty_case_id) REFERENCES warranty_cases(id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'inventory_transfers_return_case_id_fkey'
  ) THEN
    ALTER TABLE inventory_transfers
      ADD CONSTRAINT inventory_transfers_return_case_id_fkey
      FOREIGN KEY (return_case_id) REFERENCES return_cases(id);
  END IF;
END $$;

ALTER TABLE warranty_cases ENABLE ROW LEVEL SECURITY;
ALTER TABLE warranty_cases FORCE ROW LEVEL SECURITY;
ALTER TABLE return_cases ENABLE ROW LEVEL SECURITY;
ALTER TABLE return_cases FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON warranty_cases;
CREATE POLICY org_isolation ON warranty_cases
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

DROP POLICY IF EXISTS org_isolation ON return_cases;
CREATE POLICY org_isolation ON return_cases
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.slip.ship', 'inventory', 'ship', 'shipping_slip'),
  ('inventory.slip.receive', 'inventory', 'receive', 'shipping_slip'),
  ('inventory.slip.cancel', 'inventory', 'cancel', 'shipping_slip'),
  ('inventory.transport.depart', 'inventory', 'depart', 'transport_sheet'),
  ('inventory.transport.deliver', 'inventory', 'deliver', 'transport_sheet'),
  ('inventory.transport.cancel', 'inventory', 'cancel', 'transport_sheet'),
  ('inventory.parcel.read', 'inventory', 'read', 'parcel'),
  ('inventory.warranty.read', 'inventory', 'read', 'warranty_case'),
  ('inventory.warranty.create', 'inventory', 'create', 'warranty_case'),
  ('inventory.warranty.manage', 'inventory', 'manage', 'warranty_case'),
  ('inventory.return.read', 'inventory', 'read', 'return_case'),
  ('inventory.return.create', 'inventory', 'create', 'return_case'),
  ('inventory.return.manage', 'inventory', 'manage', 'return_case')
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
    'inventory.slip.ship', 'inventory.slip.receive',
    'inventory.transport.depart', 'inventory.transport.deliver',
    'inventory.parcel.read',
    'inventory.warranty.read', 'inventory.warranty.create',
    'inventory.return.read', 'inventory.return.create'
  )
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN (
  'warehouse_manager', 'regional_manager', 'store_owner', 'platform_admin'
)
  AND p.code IN (
    'inventory.slip.cancel', 'inventory.transport.cancel',
    'inventory.warranty.manage', 'inventory.return.manage'
  )
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE warranty_cases TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE return_cases TO nexus;

COMMENT ON COLUMN shipping_slips.parcel_kind IS
  'TRANSFER | CEDI_DISTRIBUTION | DEFECTIVE | WARRANTY | RETURN_TO_CEDI | RETURN_TO_VENDOR | REPAIR_OUT | REPAIR_IN';
COMMENT ON TABLE warranty_cases IS
  'Casos de garantía / reparación enviados a CEDI o centro de servicio.';
COMMENT ON TABLE return_cases IS
  'Devoluciones / defectuosos hacia CEDI o almacén de cuarentena.';
