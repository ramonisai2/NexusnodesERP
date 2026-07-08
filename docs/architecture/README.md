# NexusnodesERP — Arquitectura Técnica Empresarial

> Sistema ERP Web de nivel empresarial enfocado en **Inventarios** y **Nóminas**, con seguridad RBAC+ABAC, microservicios stateless y alta disponibilidad.

| Documento | Descripción |
|-----------|-------------|
| [01 — Arquitectura General](./01-arquitectura-general.md) | Modelo cliente-servidor, microservicios, HA/FT |
| [02 — Seguridad RBAC+ABAC](./02-seguridad-rbac-abac.md) | Control de acceso, JWT/OAuth2/MFA, nodos UI |
| [03 — Modelo Entidad-Relación](./03-modelo-entidad-relacion.md) | ER de Inventario, Nómina y Seguridad |
| [04 — Stack Tecnológico](./04-stack-tecnologico.md) | Tecnologías recomendadas y justificación |
| [05 — Persistencia e Integridad](./05-persistencia-integridad.md) | Transacciones, concurrencia, replicación |
| [Diagramas](../diagrams/) | Mermaid / C4 / ER visuales |

---

## Principios de diseño

1. **Zero Trust** — autenticación y autorización en cada hop (gateway → servicio → dato).
2. **Stateless backends** — sesión en JWT + Redis; cualquier nodo puede atender cualquier request.
3. **Consistencia fuerte** donde importa (nómina, stock) y eventual donde no (auditoría, reportes).
4. **Defense in depth** — WAF, mTLS interno, cifrado en tránsito y en reposo, secretos en vault.
5. **Observabilidad first** — traces, métricas y logs correlacionados desde el día 1.
