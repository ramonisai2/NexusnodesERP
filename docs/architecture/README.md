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
| [16 — Tempo + Grafana](./16-tempo-grafana.md) | Trazas OTLP, dashboards TraceQL |
| [17 — Instalación tienda pequeña](./17-store-setup-wizard.md) | Wizard abarrotes + perfil dueño |
| [18 — Subida móvil por QR](./18-qr-mobile-upload.md) | Teléfono escanea QR y elige fotos sin login |
| [19 — Motor de búsqueda BM25](./19-search-engine.md) | Índice invertido (estilo buscador), no scan de tablas |
| [20 — CEDI y recepción](./20-cedi-inbound-receiving.md) | Factura, etiquetas y almacén de llegada |
| [21 — Easter eggs héroes](./21-hero-easter-eggs.md) | Aliases con justificación lógica |
| [22 — Operadores compartidos y jefes](./22-shared-operators-approvals.md) | Misma cuenta, estaciones concurrentes, cola de aprobación |
| [23 — Papeletas entre tiendas](./23-shipping-slips.md) | Papelería: notas imprimibles con tipo de contenedor |
| [24 — Hojas de transporte](./24-transport-sheets.md) | Manifiesto con notas de diversos departamentos |
| [25 — Correo interno mini-Outlook](./25-messaging-outlook.md) | Bandejas, prioridades por color, anuncios con caducidad |
| [26 — Traslados de stock](./26-inventory-transfers.md) | TRANSFER_OUT/IN con ciclo tránsito → recibido |
| [27 — Tienda en línea configurable](./27-configurable-storefront.md) | Vitrina pública /tienda/{slug} + admin |
| [28 — Paquetería CEDI / garantías](./28-parcel-logistics.md) | Hub entre tiendas, defectuosos, garantías y centros |
| [29 — Cuentas de cliente y tarjetas](./29-customer-accounts-cards.md) | Registro + tarjetas chip / código de barras |
| [30 — Hardening HTTP + anti-inyección](./30-security-hardening.md) | Headers, body limit, sanitización XSS/SQLi |
| [31 — Jefes multi-departamento](./31-department-managers.md) | Un jefe, varios deptos sin relación requerida |
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
