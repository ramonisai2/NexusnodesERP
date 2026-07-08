# 07 — Outbox→NATS, Claims Enrichment y RLS

## Cambios

| Área | Detalle |
|---|---|
| Outbox relay | `apps/outbox-relay` publica eventos pendientes a NATS (`SKIP LOCKED`) |
| Eventos | `InventoryMoved`, `PayrollRunPrepared`, `PayrollRunApproved` |
| Claims enrichment | Gateway carga roles/permisos/sucursales/attrs desde BD (`user_roles`, etc.) |
| RLS | Migración `006_rls.sql` + `SET app.org_id` por transacción (`packages/go/db`) |
| NATS | `docker-compose` service `nats` o binario local |

## Arranque

```bash
export DATABASE_URL='postgres://nexus:nexus@127.0.0.1:5432/nexus_erp?sslmode=disable'
export OPA_URL='http://127.0.0.1:8181'
export NATS_URL='nats://127.0.0.1:4222'
export DEV_AUTH_BYPASS=true

# aplicar solo si falta 006
psql "$DATABASE_URL" -f infra/postgres/migrations/006_rls.sql

# NATS (sin Docker):
#   /tmp/nats-server -js
# o: docker compose up -d nats

make run-inventory
make run-payroll
make run-gateway      # requiere DATABASE_URL para enrichment
make run-relay
make run-web
```

## Verificación rápida

1. `POST /auth/dev-token?persona=analyst` → claims enriquecidos desde BD (aunque el JWT lean no traiga roles).
2. Movimiento de inventario → fila en `outbox` → relay marca `published_at` y publica `nexus.events.InventoryMoved`.
3. RLS: con `app.org_id` de otro tenant, `branches` queda vacío.

```bash
make test-go
make test-go-pg
```

## Siguiente

- ~~Consumers / OTel / Keycloak mappers~~ → ver [08-consumers-otel-keycloak.md](./08-consumers-otel-keycloak.md)
- Reporting BFF, email real, dashboards
