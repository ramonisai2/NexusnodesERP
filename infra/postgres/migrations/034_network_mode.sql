-- 034_network_mode.sql
-- Differentiate intranet-only ERP vs installs with public internet egress.

SELECT set_config('app.rls_bypass', 'on', false);

ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS network_mode TEXT NOT NULL DEFAULT 'intranet';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'organizations_network_mode_check'
  ) THEN
    ALTER TABLE organizations
      ADD CONSTRAINT organizations_network_mode_check
      CHECK (network_mode IN ('intranet', 'internet'));
  END IF;
END $$;

COMMENT ON COLUMN organizations.network_mode IS
  'intranet = LAN/VPN only (no public storefront/customer portal); internet = public egress surfaces allowed';

-- DEMO / sample org keeps public storefront and customer portal available.
UPDATE organizations
SET network_mode = 'internet'
WHERE code = 'DEMO';

-- Legacy enterprise orgs without an explicit choice stay intranet-safe by default
-- unless they already unlocked the public storefront module.
UPDATE organizations o
SET network_mode = 'internet'
WHERE o.network_mode = 'intranet'
  AND o.enabled_modules IS NOT NULL
  AND o.enabled_modules @> '"storefront"'::jsonb
  AND o.code <> 'DEMO';

UPDATE app_install
SET meta = COALESCE(meta, '{}'::jsonb) || jsonb_build_object('network_mode', 'internet')
WHERE id = 1
  AND EXISTS (SELECT 1 FROM organizations WHERE code = 'DEMO');
