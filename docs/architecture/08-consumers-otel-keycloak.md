# 08 — Consumers, OpenTelemetry y Keycloak mappers

## Cambios

| Área | Detalle |
|---|---|
| Notification consumer | `apps/notification` suscribe NATS → `notifications` + `audit_log` + `processed_events` |
| OpenTelemetry | `packages/go/otelx` (`OTEL_EXPORTER=none\|stdout\|otlp`) en gateway, inventory, payroll, relay, notification |
| Keycloak | Protocol mappers para `org_id`, `branch_ids`, `permissions`, `roles`, `attrs.*`, audience `nexus-api` |
| Migración | `007_notifications.sql` |

## Arranque

```bash
export DATABASE_URL='postgres://nexus:nexus@127.0.0.1:5432/nexus_erp?sslmode=disable'
export NATS_URL='nats://127.0.0.1:4222'
export OPA_URL='http://127.0.0.1:8181'
export OTEL_EXPORTER=none   # o stdout / otlp

psql "$DATABASE_URL" -f infra/postgres/migrations/007_notifications.sql

make run-inventory
make run-payroll
make run-gateway
make run-relay
make run-notification
```

## Flujo de eventos

```
POST movement/payroll
  → outbox row
  → outbox-relay → NATS nexus.events.<Type>
  → notification consumer
      → notifications (proyección)
      → audit_log
      → processed_events (idempotencia)
```

## Keycloak claims

Usuarios demo: `admin` / `analyst` / `approver` con atributos alineados al contrato JWT.
En runtime el gateway **también** enriquece desde BD (fuente de verdad de permisos efectivos).

## OTel

- `OTEL_EXPORTER=stdout` — spans en consola (local)
- `OTEL_EXPORTER=otlp` + `OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318` — collector
- Default `none` para no ruidoso en tests

## Siguiente

- Reporting BFF GraphQL
- Canal email real (SMTP/Mailhog)
- Dashboards Grafana / Tempo
