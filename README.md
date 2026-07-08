# NexusnodesERP

ERP Web empresarial enfocado en **inventarios** y **nóminas**, con arquitectura de microservicios stateless, seguridad RBAC+ABAC (OAuth2/OIDC + MFA) y persistencia transaccional de alta disponibilidad.

## Documentación de arquitectura

La especificación técnica completa está en [`docs/architecture/`](./docs/architecture/README.md):

| Documento | Contenido |
|-----------|-----------|
| [Arquitectura general](./docs/architecture/01-arquitectura-general.md) | Cliente-servidor, microservicios, HA/FT, flujos |
| [Seguridad RBAC+ABAC](./docs/architecture/02-seguridad-rbac-abac.md) | JWT, OAuth2, MFA, nodos UI dinámicos |
| [Modelo ER](./docs/architecture/03-modelo-entidad-relacion.md) | Inventario, nómina y seguridad por roles |
| [Stack tecnológico](./docs/architecture/04-stack-tecnologico.md) | Tecnologías recomendadas |
| [Persistencia](./docs/architecture/05-persistencia-integridad.md) | Replicación, concurrencia, sync async |
| [Diagramas](./docs/diagrams/architecture-diagrams.md) | Componentes, authz, estados, HA |

## Stack (resumen)

- **Frontend:** React 19 + TypeScript + Vite (SPA)
- **Backend:** Go + Spring Boot 3 (Kotlin) — microservicios
- **API:** REST (comandos) + GraphQL BFF (lecturas)
- **Auth:** Keycloak/OIDC + OPA (RBAC+ABAC) + MFA
- **Datos:** PostgreSQL 16 + Redis + Kafka
- **Runtime:** Kubernetes multi-AZ + service mesh

## Estado del repositorio

Fase actual: **diseño de arquitectura**. La implementación de servicios se añadirá en iteraciones posteriores.
