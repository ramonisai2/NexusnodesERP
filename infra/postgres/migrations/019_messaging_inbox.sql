-- 019_messaging_inbox.sql
-- Mini-Outlook interno: usuarios, mensajes, bandejas, prioridades y anuncios con caducidad.

SELECT set_config('app.rls_bypass', 'on', false);

CREATE TABLE IF NOT EXISTS mail_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  from_sub TEXT NOT NULL,
  from_operator TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  priority TEXT NOT NULL DEFAULT 'NORMAL'
    CHECK (priority IN ('LOW', 'NORMAL', 'HIGH', 'URGENT')),
  kind TEXT NOT NULL DEFAULT 'MESSAGE'
    CHECK (kind IN ('MESSAGE', 'ANNOUNCEMENT')),
  expires_at TIMESTAMPTZ,
  color_token TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS mail_messages_org_created_idx
  ON mail_messages (org_id, created_at DESC);

CREATE INDEX IF NOT EXISTS mail_messages_announcements_idx
  ON mail_messages (org_id, kind, expires_at)
  WHERE kind = 'ANNOUNCEMENT';

CREATE TABLE IF NOT EXISTS mail_recipients (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  message_id UUID NOT NULL REFERENCES mail_messages(id) ON DELETE CASCADE,
  org_id UUID NOT NULL REFERENCES organizations(id),
  to_sub TEXT NOT NULL,
  folder TEXT NOT NULL DEFAULT 'INBOX'
    CHECK (folder IN ('INBOX', 'SENT', 'ARCHIVE')),
  read_at TIMESTAMPTZ,
  archived_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (message_id, to_sub, folder)
);

CREATE INDEX IF NOT EXISTS mail_recipients_inbox_idx
  ON mail_recipients (org_id, to_sub, folder, created_at DESC);

CREATE INDEX IF NOT EXISTS mail_recipients_unread_idx
  ON mail_recipients (org_id, to_sub, folder)
  WHERE read_at IS NULL AND folder = 'INBOX';

COMMENT ON TABLE mail_messages IS
  'Mensajes internos (mini-Outlook): correo entre usuarios y anuncios con caducidad.';
COMMENT ON COLUMN mail_messages.priority IS
  'LOW=gris, NORMAL=azul, HIGH=ámbar, URGENT=rojo';
COMMENT ON COLUMN mail_messages.expires_at IS
  'Caducidad de anuncios; NULL = sin caducidad (mensajes normales).';

ALTER TABLE mail_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_messages FORCE ROW LEVEL SECURITY;
ALTER TABLE mail_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_recipients FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_isolation ON mail_messages;
CREATE POLICY org_isolation ON mail_messages
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

DROP POLICY IF EXISTS org_isolation ON mail_recipients;
CREATE POLICY org_isolation ON mail_recipients
  FOR ALL
  USING (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()))
  WITH CHECK (app_rls_bypass() OR (org_id IS NOT NULL AND org_id::text = app_org_id()));

INSERT INTO permissions (code, module, action, resource) VALUES
  ('mail.read', 'mail', 'read', 'message'),
  ('mail.send', 'mail', 'send', 'message'),
  ('mail.announce', 'mail', 'announce', 'announcement')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN (
  'inventory_clerk', 'payroll_analyst', 'payroll_approver',
  'warehouse_manager', 'regional_manager', 'store_owner', 'platform_admin'
)
  AND p.code IN ('mail.read', 'mail.send')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code IN (
  'warehouse_manager', 'regional_manager', 'store_owner',
  'platform_admin', 'payroll_approver'
)
  AND p.code = 'mail.announce'
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE mail_messages TO nexus;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE mail_recipients TO nexus;

-- Demo seed: welcome announcement (expires in 30 days) for org DEMO.
INSERT INTO mail_messages (
  id, org_id, from_sub, from_operator, subject, body, priority, kind, expires_at, color_token
)
SELECT
  'aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0001',
  o.id,
  'system',
  '',
  'Bienvenido al correo interno NexusERP',
  'Este es el mini-Outlook de la organización: bandeja de entrada, prioridades por color y anuncios con caducidad.',
  'HIGH',
  'ANNOUNCEMENT',
  now() + interval '30 days',
  'amber'
FROM organizations o
WHERE o.code = 'DEMO'
ON CONFLICT (id) DO NOTHING;

INSERT INTO mail_recipients (message_id, org_id, to_sub, folder)
SELECT
  'aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0001',
  o.id,
  u.idp_sub,
  'INBOX'
FROM organizations o
JOIN users u ON u.org_id = o.id
WHERE o.code = 'DEMO'
ON CONFLICT DO NOTHING;
