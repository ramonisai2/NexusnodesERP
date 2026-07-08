# 14 — Correo real con Mailhog (SMTP)

## Objetivo

El consumer de notificaciones envía **email real por SMTP** (Mailhog en local) además de persistir la proyección en `notifications` / `audit_log`.

## Eventos

| Evento NATS | Asunto |
|---|---|
| `InventoryMoved` | Inventario: movimiento registrado |
| `InventoryMovedVoided` | Inventario: movimiento revertido |
| `PayrollRunPrepared` | Nómina: corrida preparada |
| `PayrollRunApproved` | Nómina: corrida aprobada |

## Config

```bash
SMTP_ENABLED=true
SMTP_HOST=127.0.0.1
SMTP_PORT=1025
SMTP_FROM=nexus@demo.local
NOTIFY_EMAIL_TO=ops@demo.nexus,analyst@demo.nexus
```

`SMTP_ENABLED=false` → solo log (sin SMTP).

## Arranque

```bash
docker compose up -d mailhog nats postgres
# UI Mailhog: http://localhost:8025

make run-inventory
make run-payroll
make run-relay
make run-notification
make run-gateway
```

Flujo:

```
acción de negocio → outbox → relay → NATS
  → notification consumer
      → SMTP (Mailhog :1025)
      → notifications (channel=email)
      → audit_log + processed_events
```

## Idempotencia

1. `QueueSubscribe` en el grupo `notification` (una instancia procesa cada mensaje).
2. Claim en `processed_events (event_id, consumer)` con `ON CONFLICT DO NOTHING` **antes** de SMTP.
3. El envío SMTP ocurre **fuera** de la transacción; luego se persiste `notifications` + `audit_log`.

## Verificación

1. Preparar/aprobar nómina o registrar/revertir un movimiento.
2. Abrir http://localhost:8025 y ver el correo HTML.
3. `SELECT channel, status, subject FROM notifications ORDER BY created_at DESC LIMIT 5;`

Sin Docker: binario MailHog en `:1025` / UI `:8025` (mismo SMTP que compose).
