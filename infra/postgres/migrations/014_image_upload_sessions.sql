-- 014_image_upload_sessions.sql
-- Short-lived tokens so a phone can upload report photos via QR (no JWT on device).

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS image_upload_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash TEXT NOT NULL UNIQUE,
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id TEXT NOT NULL, -- branch code as used by APIs (e.g. br_norte / tienda)
  created_by TEXT NOT NULL,
  title_hint TEXT NOT NULL DEFAULT '',
  max_files INT NOT NULL DEFAULT 8 CHECK (max_files > 0 AND max_files <= 20),
  uploads_count INT NOT NULL DEFAULT 0 CHECK (uploads_count >= 0),
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS image_upload_sessions_expires_idx
  ON image_upload_sessions (expires_at)
  WHERE revoked_at IS NULL;

ALTER TABLE image_upload_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE image_upload_sessions FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON image_upload_sessions;
CREATE POLICY org_isolation ON image_upload_sessions
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE image_upload_sessions TO nexus;
