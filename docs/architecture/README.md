# NexusERP — Arquitectura Técnica Empresarial

> Sistema ERP Web de nivel empresarial enfocado en **Inventarios** y **Nóminas**, diseñado bajo principios de alta disponibilidad, seguridad Zero-Trust y consistencia transaccional.

| Atributo | Valor |
|---|---|
| Estilo | Cliente–Servidor distribuido (Microservicios Stateless) |
| APIs | REST + GraphQL (BFF) protegidas |
| AuthN/AuthZ | OAuth2 + OIDC + MFA · JWT · RBAC + ABAC |
| Persistencia | PostgreSQL (OLTP) + réplicas · Event Store · Caché |
| Disponibilidad | Multi-AZ · Load Balancing · Circuit Breakers · Retry/Backoff |

---

## Índice de documentos

| Documento | Contenido |
|---|---|
| [01 — Arquitectura de componentes](./01-component-architecture.md) | Capas, microservicios, HA, flujos |
| [02 — Seguridad RBAC/ABAC](./02-security-rbac-abac.md) | Modelo de acceso, JWT, MFA, nodos UI |
| [03 — Persistencia e integridad](./03-persistence-integrity.md) | Transacciones, concurrencia, sync async |
| [04 — Stack tecnológico](./04-tech-stack.md) | Tecnologías recomendadas y justificación |
| [05 — Bootstrap local](./05-bootstrap-guide.md) | Monorepo, auth dev, cómo correr |
| [06 — Postgres + JWKS + OPA](./06-postgres-jwks-opa.md) | Persistencia real, PEP→OPA, JWKS |
| [07 — Outbox + Claims + RLS](./07-outbox-claims-rls.md) | NATS relay, enrichment BD, RLS |
| [08 — Consumers + OTel + Keycloak](./08-consumers-otel-keycloak.md) | Notificaciones, traces, mappers |
| [09 — UX e i18n ES/EN](./09-ux-i18n.md) | Copy amigable y selector de idioma |
| [10 — Jerarquía de jefes y reversión](./10-manager-hierarchy-revert.md) | Jefe de área / regional y void de movimientos |
| [11 — Departamentos y multi-colocación](./11-store-departments.md) | Electrónica/Juguetería; artículo en varios deptos |
| [12 — Etiquetadora y precios por tienda](./12-store-labels-pricing.md) | Descripción pública, material, barcode, precios |
| [13 — Reporting BFF GraphQL](./13-reporting-bff-graphql.md) | Agregaciones de solo lectura para la SPA |
| [14 — Correo SMTP / Mailhog](./14-mailhog-email.md) | Notificaciones por email en local |
| [15 — Reportes con imágenes](./15-image-reports.md) | Upload con downscale web-friendly |
| [Diagrama de arquitectura](../diagrams/architecture-overview.mmd) | Mermaid — vista de componentes |
| [Diagrama de flujo auth](../diagrams/auth-flow.mmd) | Mermaid — OAuth2/MFA/JWT |
| [Diagrama de flujo inventario](../diagrams/inventory-flow.mmd) | Mermaid — movimiento de stock |
| [Diagrama de flujo nómina](../diagrams/payroll-flow.mmd) | Mermaid — ciclo de nómina |
| [Modelo ER](../data-model/er-inventory-payroll.md) | Entidades, relaciones, seguridad |

---

## Principios de diseño

1. **Stateless en el edge**: ningún microservicio de negocio mantiene sesión en memoria; el estado vive en JWT + Redis + BD.
2. **Separación de lecturas/escrituras**: CQRS ligero en Inventario y Nómina (comandos transaccionales vs. proyecciones de consulta).
3. **Seguridad por defecto**: deny-all; cada endpoint y cada nodo UI se evalúan con políticas RBAC+ABAC.
4. **Consistencia fuerte donde importa**: nóminas e inventarios usan transacciones ACID; eventos asíncronos para proyecciones y auditoría.
5. **Observabilidad first-class**: traces, métricas y logs correlacionados por `correlationId` / `tenantId` / `branchId`.
6. **Multi-sucursal / multi-tenant lógico**: aislamiento por `organization_id` + `branch_id` en todas las consultas y políticas ABAC.
