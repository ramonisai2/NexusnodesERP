-- 021_storefront_settings.sql
-- Tienda en línea configurable por sucursal (catálogo público desde etiquetas/precios).

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS branch_storefront_settings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  public_slug TEXT NOT NULL,
  published BOOLEAN NOT NULL DEFAULT FALSE,
  brand_name TEXT NOT NULL DEFAULT '',
  tagline TEXT NOT NULL DEFAULT '',
  primary_color TEXT NOT NULL DEFAULT '#1a5c3a',
  accent_color TEXT NOT NULL DEFAULT '#c6f2a8',
  hero_title TEXT NOT NULL DEFAULT '',
  hero_subtitle TEXT NOT NULL DEFAULT '',
  hero_image_url TEXT NOT NULL DEFAULT '',
  cta_label TEXT NOT NULL DEFAULT '',
  cta_url TEXT NOT NULL DEFAULT '',
  show_prices BOOLEAN NOT NULL DEFAULT TRUE,
  show_stock_badge BOOLEAN NOT NULL DEFAULT TRUE,
  in_stock_only BOOLEAN NOT NULL DEFAULT TRUE,
  featured_skus TEXT[] NOT NULL DEFAULT '{}',
  contact_phone TEXT NOT NULL DEFAULT '',
  contact_whatsapp TEXT NOT NULL DEFAULT '',
  contact_email TEXT NOT NULL DEFAULT '',
  contact_address TEXT NOT NULL DEFAULT '',
  contact_hours TEXT NOT NULL DEFAULT '',
  maps_url TEXT NOT NULL DEFAULT '',
  updated_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (branch_id),
  UNIQUE (public_slug)
);

CREATE INDEX IF NOT EXISTS branch_storefront_published_idx
  ON branch_storefront_settings (public_slug)
  WHERE published = TRUE;

COMMENT ON TABLE branch_storefront_settings IS
  'Configuración de vitrina pública /tienda/{slug}: marca, hero, contacto y flags de catálogo.';

ALTER TABLE branch_storefront_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE branch_storefront_settings FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON branch_storefront_settings;
CREATE POLICY org_isolation ON branch_storefront_settings
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

INSERT INTO permissions (code, module, action, resource) VALUES
  ('store.storefront.read', 'store', 'read', 'storefront'),
  ('store.storefront.manage', 'store', 'manage', 'storefront')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN ('store_owner', 'platform_admin', 'regional_manager', 'warehouse_manager')
  AND p.code IN ('store.storefront.read', 'store.storefront.manage')
ON CONFLICT DO NOTHING;

-- Also let inventory clerks read settings (not publish).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'inventory_clerk'
  AND p.code = 'store.storefront.read'
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE branch_storefront_settings TO nexus;

-- Seed a draft storefront for DEMO org / br_norte (unpublished until owner publishes).
INSERT INTO branch_storefront_settings (
  org_id, branch_id, public_slug, published, brand_name, tagline,
  hero_title, hero_subtitle, cta_label, show_prices, show_stock_badge, in_stock_only,
  featured_skus, contact_phone, contact_hours
)
SELECT
  o.id,
  b.id,
  'demo-norte',
  FALSE,
  COALESCE(o.name, 'Nexus Demo'),
  'Catálogo de la sucursal Norte',
  'Lo que hay hoy en tienda',
  'Precios y existencias de tu sucursal. Contáctanos para apartar.',
  'Ver catálogo',
  TRUE,
  TRUE,
  TRUE,
  ARRAY['BOLT-M8', 'PHONE-X', 'LAPTOP-14']::text[],
  '',
  'Lun–Sáb 9:00–20:00'
FROM organizations o
JOIN branches b ON b.org_id = o.id AND b.code = 'br_norte'
WHERE o.code = 'DEMO'
ON CONFLICT (branch_id) DO NOTHING;
