# 25 — Correo interno (mini-Outlook)

## Objetivo

Un **correo interno** entre usuarios de la organización (no SMTP externo). Incluye:

- Bandejas: **Entrada**, **Enviados**, **Archivo**
- Prioridades con color: LOW gris · NORMAL azul · HIGH ámbar · URGENT rojo
- **Anuncios** con caducidad (`expires_at`); al vencer dejan de aparecer en la bandeja

Mailhog / `apps/notification` siguen siendo solo para correo SMTP de eventos.

```
SPA /mail
  → gateway /mail/*
  → apps/messaging :8087
  → Postgres mail_messages + mail_recipients
```

## Modelo

| Tabla | Rol |
|---|---|
| `mail_messages` | Cabecera: asunto, cuerpo, prioridad, kind, caducidad |
| `mail_recipients` | Copia por usuario + bandeja (`INBOX` / `SENT` / `ARCHIVE`) |

Permisos: `mail.read`, `mail.send`, `mail.announce`.

## API (vía gateway)

| Método | Ruta | Permiso |
|---|---|---|
| `GET /mail/inbox` | bandeja de entrada | `mail.read` |
| `GET /mail/sent` | enviados | `mail.read` |
| `GET /mail/archive` | archivo | `mail.read` |
| `GET /mail/unread-count` | no leídos | `mail.read` |
| `GET /mail/directory` | usuarios de la org | `mail.read` |
| `GET /mail/messages/{id}` | detalle | `mail.read` |
| `POST /mail/messages` | enviar / anunciar | `mail.send` (+ `mail.announce`) |
| `POST /mail/messages/{id}/read` | marcar leído | `mail.read` |
| `POST /mail/messages/{id}/archive` | archivar | `mail.read` |

Body de envío (ejemplo):

```json
{
  "subject": "Cierre de caja",
  "body": "Recuerden cuadrar antes de las 21:00",
  "priority": "HIGH",
  "kind": "MESSAGE",
  "to_subs": ["usr_dev_analyst"]
}
```

Anuncio a toda la org (sin `to_subs`, caduca en N días):

```json
{
  "subject": "Mantenimiento domingo",
  "body": "El sistema estará en pausa de 2 a 4 AM.",
  "priority": "URGENT",
  "kind": "ANNOUNCEMENT",
  "expires_at": "2026-07-15T00:00:00Z"
}
```

## SPA

Menú **Correo** → `/mail`

1. Cambia de bandeja (entrada / enviados / archivo).
2. Abre un mensaje (se marca leído en entrada).
3. **Redactar**: elige destinatarios, prioridad y, si tienes permiso, anuncio con vigencia.

## Migración

`infra/postgres/migrations/019_messaging_inbox.sql`

## Local

```bash
make migrate
make run-messaging   # :8087
make run-gateway     # proxy /mail → messaging
```

Variables: `MESSAGING_URL=http://127.0.0.1:8087`, `MESSAGING_ADDR=:8087`.
