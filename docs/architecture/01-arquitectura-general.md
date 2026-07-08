# 01 — Arquitectura General

## 1. Visión

Arquitectura **cliente-servidor distribuida** con:

- Frontend SPA (cliente)
- API Gateway + BFF
- Backend de **microservicios stateless**
- Comunicación **REST + GraphQL** (GraphQL en BFF de lectura; REST para comandos/escritura)
- Alta disponibilidad con balanceo L4/L7 y multi-AZ

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLIENTES                                        │
│   SPA (React/Next)  │  Mobile (futuro)  │  Integraciones (Webhooks/API)     │
└─────────────┬───────────────────┬───────────────────────┬───────────────────┘
              │ HTTPS/TLS 1.3     │                       │
              ▼                   ▼                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  EDGE: CDN + WAF + DDoS (Cloudflare / AWS CloudFront + WAF)                 │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  LOAD BALANCER (L7) — ALB / NGINX Ingress / Traefik                         │
│  Health checks · Sticky opcional · TLS termination · Rate limiting          │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  API GATEWAY + BFF                                                          │
│  · AuthN (OAuth2/OIDC) · JWT validation · Routing · GraphQL federation      │
│  · Request ID · Circuit breaker · Quota por tenant/sucursal                 │
└───────┬─────────────┬─────────────┬─────────────┬─────────────┬─────────────┘
        │             │             │             │             │
        ▼             ▼             ▼             ▼             ▼
   ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
   │ Identity│  │Inventory│  │ Payroll │  │ Catalog │  │ Audit   │
   │ & Access│  │  MS     │  │  MS     │  │  MS     │  │  MS     │
   └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘
        │            │            │            │            │
        └────────────┴─────┬──────┴────────────┴────────────┘
                           │  Event Bus (Kafka / NATS JetStream)
                           ▼
              ┌────────────────────────────┐
              │  Async workers / Outbox    │
              │  Sync · Notificaciones ·   │
              │  Reportes · Réplicas CQRS  │
              └────────────────────────────┘
```

## 2. Capas

| Capa | Responsabilidad |
|------|-----------------|
| **Presentación** | SPA, renderizado condicional por permisos, estado global |
| **Edge** | CDN, WAF, DDoS, TLS, geo-routing |
| **Gateway/BFF** | AuthN, agregación GraphQL, rate limit, correlación |
| **Dominio** | Microservicios por bounded context (DDD) |
| **Integración** | Event bus, outbox, webhooks, ETL |
| **Datos** | PostgreSQL primario + réplicas, Redis, object storage |
| **Plataforma** | K8s, service mesh, secrets, observabilidad |

## 3. Microservicios (Bounded Contexts)

| Servicio | Dominio | API | Persistencia |
|----------|---------|-----|--------------|
| `identity-service` | Usuarios, roles, políticas ABAC, MFA, sesiones | REST | PostgreSQL + Redis |
| `inventory-service` | Stock, movimientos, almacenes, reservas | REST + eventos | PostgreSQL |
| `payroll-service` | Empleados, periodos, liquidaciones, pagos | REST + eventos | PostgreSQL |
| `catalog-service` | Productos, SKUs, unidades, impuestos | REST/GraphQL | PostgreSQL |
| `org-service` | Empresas, sucursales, centros de costo | REST | PostgreSQL |
| `audit-service` | Trail inmutable, compliance | Eventos + REST lectura | PostgreSQL (append-only) / ClickHouse |
| `notification-service` | Email, push, in-app | Eventos | Redis + cola |
| `reporting-service` | Reportes, dashboards | GraphQL | Réplica read + warehouse |

### Reglas de comunicación

- **Síncrono (REST)**: comandos que requieren respuesta inmediata (crear movimiento, liquidar nómina).
- **GraphQL (BFF)**: consultas agregadas multi-servicio para la SPA (menos round-trips).
- **Asíncrono (eventos)**: side-effects (auditoría, notificaciones, proyecciones CQRS, sync entre sucursales).
- **Prohibido**: llamadas síncronas en cadena > 2 hops (evitar cascadas). Usar saga/orquestación o coreografía.

## 4. Alta disponibilidad y tolerancia a fallos

```
                    ┌──────────────┐
                    │ Global LB /  │
                    │ DNS failover │
                    └──────┬───────┘
           ┌───────────────┼───────────────┐
           ▼               ▼               ▼
        AZ-a            AZ-b            AZ-c
     ┌─────────┐     ┌─────────┐     ┌─────────┐
     │ Pods xN │     │ Pods xN │     │ Pods xN │
     │ + Redis │     │ + Redis │     │ (standby│
     │ replica │     │ replica │     │  cold)  │
     └────┬────┘     └────┬────┘     └────┬────┘
          │               │               │
          └───────────────┼───────────────┘
                          ▼
              PostgreSQL Primary (AZ-a)
              Sync replica (AZ-b)
              Async replica (AZ-c) + PITR
```

### Mecanismos

| Mecanismo | Implementación |
|-----------|----------------|
| Balanceo | ALB/NGINX Ingress, least connections + health probes |
| Stateless | Sin sticky session obligatoria; JWT + Redis session store |
| Circuit breaker | Resilience4j / Envoy outlier detection |
| Retry | Idempotency-Key en escrituras; backoff exponencial |
| Bulkhead | Límites de conexión por servicio en mesh |
| Graceful shutdown | PreStop + drain de conexiones |
| Multi-AZ | Mínimo 2 AZ activas; RPO ≈ 0 en sync replica |
| Chaos | Pruebas periódicas (pod kill, network partition) |

## 5. Flujo de request típico (escritura de inventario)

```mermaid
sequenceDiagram
    participant SPA
    participant GW as API Gateway
    participant IAM as Identity
    participant INV as Inventory
    participant DB as PostgreSQL
    participant BUS as Event Bus
    participant AUD as Audit

    SPA->>GW: POST /inventory/movements<br/>Authorization: Bearer JWT<br/>Idempotency-Key
    GW->>GW: Validar JWT, rate limit, WAF
    GW->>IAM: Introspect / PDP (RBAC+ABAC)
    IAM-->>GW: Permit (branch=MX-01, action=stock.adjust)
    GW->>INV: Forward + X-User-Context
    INV->>INV: Validar política local + versión optimistic lock
    INV->>DB: BEGIN; UPDATE stock WHERE version=N; INSERT movement; COMMIT
    INV->>BUS: Publish InventoryMoved (outbox)
    INV-->>SPA: 201 Created
    BUS->>AUD: Append audit event
```

## 6. Despliegue

- **Orquestación**: Kubernetes (EKS/GKE/AKS) + Helm/Kustomize
- **Service mesh**: Istio o Linkerd (mTLS, retries, observability)
- **CI/CD**: GitHub Actions → build → scan (SAST/SCA) → staging → canary → prod
- **Config**: 12-factor; secrets en HashiCorp Vault / AWS Secrets Manager
- **Tenancy**: multi-tenant por `tenant_id` (row-level) con opción de DB dedicada para enterprise

## 7. SLOs objetivo

| Métrica | Objetivo |
|---------|----------|
| Disponibilidad API | 99.9% mensual |
| p95 latencia lectura | < 200 ms |
| p95 latencia escritura inventario | < 400 ms |
| p95 liquidación nómina (lote 500) | < 30 s |
| RPO | ≤ 0 (sync) / ≤ 5 min (async DR) |
| RTO | ≤ 15 min |
