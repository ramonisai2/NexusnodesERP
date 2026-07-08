-- 018_transport_sheets.sql
-- Hojas de transporte: agrupan notas/papeletas de diversos departamentos
-- para un envío entre tiendas.

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS transport_sheets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  sheet_number TEXT NOT NULL,
  from_branch_id UUID NOT NULL REFERENCES branches(id),
  to_branch_id UUID NOT NULL REFERENCES branches(id),
  carrier_name TEXT NOT NULL DEFAULT '',
  vehicle_ref TEXT NOT NULL DEFAULT '',
  driver_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'DRAFT'
    CHECK (status IN ('DRAFT', 'PRINTED', 'IN_TRANSIT', 'DELIVERED', 'CANCELLED')),
  printed_at TIMESTAMPTZ,
  departed_at TIMESTAMPTZ,
  delivered_at TIMESTAMPTZ,
  created_by TEXT NOT NULL DEFAULT '',
  operator_label TEXT NOT NULL DEFAULT '',
  session_id UUID REFERENCES sessions(id),
  notes TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, idempotency_key),
  UNIQUE (org_id, sheet_number)
);

CREATE INDEX IF NOT EXISTS transport_sheets_org_status_idx
  ON transport_sheets (org_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS transport_sheets_from_branch_idx
  ON transport_sheets (from_branch_id, created_at DESC);

-- One block per department with free-form notes (papelería / instrucciones).
CREATE TABLE IF NOT EXISTS transport_sheet_sections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sheet_id UUID NOT NULL REFERENCES transport_sheets(id) ON DELETE CASCADE,
  department_code TEXT NOT NULL,
  department_name TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS transport_sheet_sections_sheet_idx
  ON transport_sheet_sections (sheet_id, sort_order);

-- Optional link from a department section to existing shipping slips (papeletas).
CREATE TABLE IF NOT EXISTS transport_sheet_slip_links (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  section_id UUID NOT NULL REFERENCES transport_sheet_sections(id) ON DELETE CASCADE,
  shipping_slip_id UUID NOT NULL REFERENCES shipping_slips(id),
  UNIQUE (section_id, shipping_slip_id)
);

CREATE INDEX IF NOT EXISTS transport_sheet_slip_links_section_idx
  ON transport_sheet_slip_links (section_id);

COMMENT ON TABLE transport_sheets IS
  'Hoja de transporte: documento que acompaña el envío y lista notas por departamento.';
COMMENT ON TABLE transport_sheet_sections IS
  'Bloque de notas de un departamento dentro de la hoja de transporte.';

ALTER TABLE transport_sheets ENABLE ROW LEVEL SECURITY;
ALTER TABLE transport_sheets FORCE ROW LEVEL SECURITY;
ALTER TABLE transport_sheet_sections ENABLE ROW LEVEL SECURITY;
ALTER TABLE transport_sheet_sections FORCE ROW LEVEL SECURITY;
ALTER TABLE transport_sheet_slip_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE transport_sheet_slip_links FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON transport_sheets;
CREATE POLICY org_isolation ON transport_sheets
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

DROP POLICY IF EXISTS org_isolation ON transport_sheet_sections;
CREATE POLICY org_isolation ON transport_sheet_sections
  FOR ALL
  USING (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM transport_sheets t
      WHERE t.id = transport_sheet_sections.sheet_id
        AND t.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass() OR EXISTS (
      SELECT 1 FROM transport_sheets t
      WHERE t.id = transport_sheet_sections.sheet_id
        AND t.org_id::text = app_org_id()
    )
  );

DROP POLICY IF EXISTS org_isolation ON transport_sheet_slip_links;
CREATE POLICY org_isolation ON transport_sheet_slip_links
  FOR ALL
  USING (
    app_rls_bypass() OR EXISTS (
      SELECT 1
      FROM transport_sheet_sections sec
      JOIN transport_sheets t ON t.id = sec.sheet_id
      WHERE sec.id = transport_sheet_slip_links.section_id
        AND t.org_id::text = app_org_id()
    )
  )
  WITH CHECK (
    app_rls_bypass() OR EXISTS (
      SELECT 1
      FROM transport_sheet_sections sec
      JOIN transport_sheets t ON t.id = sec.sheet_id
      WHERE sec.id = transport_sheet_slip_links.section_id
        AND t.org_id::text = app_org_id()
    )
  );

INSERT INTO permissions (code, module, action, resource) VALUES
  ('inventory.transport.read', 'inventory', 'read', 'transport_sheet'),
  ('inventory.transport.create', 'inventory', 'create', 'transport_sheet'),
  ('inventory.transport.print', 'inventory', 'print', 'transport_sheet')
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
    'inventory.transport.read', 'inventory.transport.create', 'inventory.transport.print'
  )
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE transport_sheets TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE transport_sheet_sections TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE transport_sheet_slip_links TO nexus;
