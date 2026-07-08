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
| Diagramas Mermaid | [docs/diagrams/](./docs/diagrams/) |
| Modelo ER Inventario + Nómina + Seguridad | [docs/data-model/er-inventory-payroll.md](./docs/data-model/er-inventory-payroll.md) |

## Licencia

Ver [LICENSE](./LICENSE).
