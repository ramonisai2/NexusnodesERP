# 04 — Stack Tecnológico Sugerido

Stack orientado a **robustez, rendimiento y operabilidad** en 2026. Alternativas equivalentes se indican donde aplica.

## Vista rápida

| Capa | Tecnología principal | Alternativa sólida |
|------|----------------------|--------------------|
| Frontend SPA | **React 19 + TypeScript + Vite** | Next.js (App Router) si SSR/SEO interno |
| Estado global | **TanStack Query + Zustand** | Redux Toolkit Query |
| UI | **React Aria / Base UI** + CSS Modules o Tailwind controlado | — |
| BFF / GraphQL | **Apollo Gateway / GraphQL Yoga + Hive** | Hasura (solo lecturas) |
| API Gateway | **Kong** o **APISIX** | AWS API Gateway + ALB |
| Microservicios | **Go** (inventory, gateway plugins) + **Java/Kotlin Spring Boot 3** (payroll) | .NET 8 |
| Auth | **Keycloak** (self-hosted) o **Auth0/Cognito** | Zitadel |
| Policy engine | **Open Policy Agent (OPA)** / **Cedar** | Keycloak Authorization Services |
| DB primaria | **PostgreSQL 16+** | — |
| Caché / sesión | **Redis 7** (Cluster) | KeyDB |
| Event bus | **Apache Kafka** (o **Redpanda**) | NATS JetStream (menor escala) |
| Object storage | **S3-compatible** | — |
| Orquestación | **Kubernetes** | ECS/Fargate (equipos pequeños) |
| Mesh | **Linkerd** o **Istio** | Cilium service mesh |
| Observabilidad | **OpenTelemetry + Grafana stack** (Loki, Tempo, Mimir/Prometheus) | Datadog |
| CI/CD | **GitHub Actions** + Argo CD | GitLab CI |
| Secrets | **HashiCorp Vault** | AWS Secrets Manager + External Secrets |
| IaC | **Terraform** + **OpenTofu** | Pulumi |

## Justificación por capa

### Frontend
- **React + TS + Vite**: ecosistema maduro, code-splitting fino, DX alta.
- **TanStack Query**: caché de servidor, revalidación, menos estado duplicado.
- **Zustand**: estado UI/sesión ligero (branch activa, manifesto de permisos).
- HTML5 semántico + virtualización (`@tanstack/react-virtual`) para grillas de inventario/nómina tipo escritorio.
- Rutas protegidas con permission guards; lazy load por módulo.

### Backend
- **Go**: alta concurrencia y bajo footprint para inventory (movimientos masivos).
- **Spring Boot 3 / Kotlin**: dominio de nómina con reglas complejas, batch, transacciones JPA/jOOQ.
- APIs **REST** versionadas (`/api/v1`) para comandos; **GraphQL** en BFF para pantallas agregadas.
- Contratos: **OpenAPI 3.1** + **AsyncAPI** para eventos.
- Idempotencia y outbox como librerías transversales.

### Datos
- **PostgreSQL**: ACID, RLS, JSONB para ABAC, particionado, logical replication.
- Réplicas de lectura para reporting; **Patron CQRS** ligero hacia ClickHouse/BigQuery para analítica.
- **Redis**: sesiones, rate limits, locks distribuidos cortos, denylist JWT.
- Migraciones: **Flyway** / **golang-migrate**.

### Mensajería
- Kafka/Redpanda: topics por dominio (`inventory.events`, `payroll.events`, `audit.events`).
- Patrón **Transactional Outbox** + Debezium o poller.
- Consumers idempotentes por `event_id`.

### Seguridad
- OIDC + PKCE, MFA TOTP/WebAuthn.
- OPA sidecar o central PDP con bundle de políticas versionadas.
- Escaneo: Trivy, Semgrep, Dependabot/Renovate, Cosign.

## Topología de runtime (prod)

```
Internet → Cloudflare/WAF → ALB → Kong → (BFF GraphQL | REST services)
                                         ↓
                              K8s Deployments (HPA)
                                         ↓
                         PostgreSQL (primary + sync replica)
                         Redis Cluster | Kafka | S3 | Vault
```

## Criterios de rendimiento

| Área | Práctica |
|------|----------|
| Inventario | Batch posting, índices covering, connection pooling (PgBouncer) |
| Nómina | Cálculo paralelo por partición de empleados; job queue |
| SPA | Route-based splitting, prefetch de manifesto, HTTP/2-3 |
| API | p95 budgets en SLOs; cache-control en catálogos |
| DB | EXPLAIN en CI para queries críticas; autovacuum tuneado |

## Qué evitar (anti-patrones)

- Monolito modular “temporal” sin boundaries claros de datos.
- GraphQL mutable sin authz por campo (usar solo lecturas agregadas o con directives estrictas).
- Shared DB entre microservicios (romper encapsulación).
- JWT de larga duración en localStorage.
- Microservicios excesivos al inicio: **empezar con 4–6 servicios** (identity, org, inventory, payroll, audit, bff) y extraer después.
