# 06 — Persistencia PostgreSQL, JWKS y PEP→OPA

## Cambios de este slice

| Área | Detalle |
|---|---|
| Inventory / Payroll stores | PostgreSQL vía `pgx` cuando `DATABASE_URL` está definido; memory como fallback |
| AuthZ compartido | `packages/go/authz` — cliente OPA + evaluación local (SoD, branch, límites) |
| DB compartida | `packages/go/db` — pool `pgxpool` |
| Gateway JWT | Valida **RS256 via JWKS** (Keycloak) y **HS256** en modo dev |
| Headers PEP | Gateway reenvía `X-User-Id`, `X-Org-Id`, `X-Branch-Ids`, `X-Roles`, `X-Permissions`, `X-Amr`, `X-Attrs-JSON` |
| Escrituras | Inventory `POST /movements` y Payroll prepare/approve consultan OPA antes de mutar |
| Migración | `005_runtime_actors.sql` — usuarios persona + SKU NUT-M8 |

## Arranque con Postgres + OPA

```bash
# Postgres local o docker compose
export DATABASE_URL='postgres://nexus:nexus@127.0.0.1:5432/nexus_erp?sslmode=disable'
export OPA_URL='http://127.0.0.1:8181'
export DEV_AUTH_BYPASS=true

make migrate
opa run --server --addr=127.0.0.1:8181 infra/opa/policies   # o docker compose up opa

make run-inventory
make run-payroll
make run-gateway
make run-web
```

## Tests

```bash
make test-go
make test-go-pg   # requiere DATABASE_URL y migraciones aplicadas
```

## JWKS (producción)

1. Keycloak emite access tokens RS256 (`kid` en header).
2. Gateway `JWT_JWKS_URL` apunta a `/realms/nexus/protocol/openid-connect/certs`.
3. Claims custom (`org_id`, `branch_ids`, `permissions`, `attrs`) deben mapearse en el IdP (protocol mapper) o enriquecerse vía `/me` desde BD.

En local, `POST /auth/dev-token` sigue emitiendo HS256 mientras `DEV_AUTH_BYPASS=true`.

## Siguiente

- ~~Outbox relay → NATS, enrichment de claims, RLS~~ → ver [07-outbox-claims-rls.md](./07-outbox-claims-rls.md)
- Consumers de eventos / OTel / mappers Keycloak
