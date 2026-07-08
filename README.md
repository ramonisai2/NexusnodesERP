# NexusERP

ERP Web empresarial enfocado en **Inventarios** y **Nóminas**, con arquitectura de microservicios stateless, seguridad RBAC+ABAC (OAuth2/MFA/JWT) y persistencia transaccional de alta disponibilidad.

## Documentación de arquitectura

La especificación técnica completa está en [`docs/architecture/`](./docs/architecture/README.md):

| Entregable | Ubicación |
|---|---|
| Arquitectura de componentes + HA | [docs/architecture/01-component-architecture.md](./docs/architecture/01-component-architecture.md) |
| Seguridad RBAC/ABAC + JWT/MFA | [docs/architecture/02-security-rbac-abac.md](./docs/architecture/02-security-rbac-abac.md) |
| Persistencia, concurrencia, sync | [docs/architecture/03-persistence-integrity.md](./docs/architecture/03-persistence-integrity.md) |
| Stack tecnológico recomendado | [docs/architecture/04-tech-stack.md](./docs/architecture/04-tech-stack.md) |
| Guía de bootstrap local | [docs/architecture/05-bootstrap-guide.md](./docs/architecture/05-bootstrap-guide.md) |
| Postgres + JWKS + OPA | [docs/architecture/06-postgres-jwks-opa.md](./docs/architecture/06-postgres-jwks-opa.md) |
| Diagramas Mermaid | [docs/diagrams/](./docs/diagrams/) |
| Modelo ER Inventario + Nómina + Seguridad | [docs/data-model/er-inventory-payroll.md](./docs/data-model/er-inventory-payroll.md) |

## Monorepo (bootstrap)

```text
apps/
  web/          SPA React + PolicyGuard
  gateway/      API Gateway (JWT + proxy)
  inventory/    Microservicio inventarios
  payroll/      Microservicio nóminas
infra/
  postgres/     Migraciones + seed
  opa/          Políticas Rego
  keycloak/     Realm de desarrollo
packages/
  api-contracts/ OpenAPI
```

```bash
cp .env.example .env
make deps && make test-go
# Con Postgres + OPA (recomendado):
#   make migrate && opa run --server --addr=127.0.0.1:8181 infra/opa/policies
make run-inventory   # :8082  (usa DATABASE_URL si está definido)
make run-payroll     # :8083
make run-gateway     # :8080  (JWKS RS256 + HS256 dev)
make run-web         # :5173
```

## Licencia

Ver [LICENSE](./LICENSE).
