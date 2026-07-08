# 04 — Stack Tecnológico Sugerido

Stack orientado a **robustez, rendimiento y operabilidad empresarial** (2025–2026). Alternativas equivalentes se indican cuando el equipo ya tiene expertise.

## 1. Resumen ejecutivo

| Capa | Tecnología primaria | Alternativa sólida |
|---|---|---|
| SPA | **React 19 + TypeScript** + Vite | Vue 3 + TypeScript |
| Estado cliente | **TanStack Query** + Zustand | Redux Toolkit Query |
| UI | Design system propio (Radix/shadcn primitives) | — |
| API Gateway | **Kong** / **Envoy Gateway** | AWS API Gateway + ALB |
| Identidad | **Keycloak** o **Auth0/Okta** + WebAuthn | FusionAuth |
| PDP (ABAC) | **Open Policy Agent (OPA/Rego)** o **Cedar** | Casbin (más simple) |
| Microservicios | **Go** o **Java 21 (Spring Boot 3)** | .NET 8 |
| APIs | REST (OpenAPI 3.1) + **GraphQL** (BFF) | gRPC interno |
| OLTP | **PostgreSQL 16+** | — |
| Caché / sesión | **Redis 7 Cluster** | KeyDB |
| Mensajería | **Apache Kafka** | NATS JetStream |
| Orquestación | **Kubernetes** (EKS/GKE/AKS) | — |
| Service Mesh | **Linkerd** o **Istio** (mTLS) | Cilium |
| Observabilidad | **OpenTelemetry** + Grafana/Tempo/Loki/Prometheus | Datadog / Elastic |
| CI/CD | GitHub Actions / GitLab CI + progressive delivery | Argo Rollouts |
| IaC | Terraform + Helm | Pulumi |
| Secretos | HashiCorp Vault / cloud KMS | External Secrets Operator |

## 2. Frontend (SPA)

**Elección: React 19 + TypeScript + Vite**

| Requisito | Cómo se cubre |
|---|---|
| SPA desktop-like | Layout persistente, paneles, atajos, virtualización de tablas |
| Render eficiente | Concurrent features, code-splitting por módulo (Inventario/Nómina) |
| Estado global | Server state en TanStack Query; UI state en Zustand |
| HTML5 semántico | Landmarks, tablas accesibles, formularios nativos + ARIA |
| Seguridad cliente | Tokens en memoria + refresh httpOnly; CSP; PolicyGuard |

**Librerías clave:** React Router, TanStack Table (grids de inventario/nómina), React Hook Form + Zod, Playwright (E2E).

## 3. Backend

### Opción A — Go (recomendado para Inventory throughput)

- Alta concurrencia, binarios pequeños, excelente para servicios de stock.
- Chi/Echo/Fiber o connect-go; sqlc + pgx.

### Opción B — Java 21 + Spring Boot 3 (recomendado para Payroll/HR)

- Madurez empresarial, transacciones, batch (Spring Batch para corridas de nómina).
- Spring Security Resource Server + OPA side-car.

**Contrato:** OpenAPI como source of truth; GraphQL solo en BFF de reporting (Apollo Federation o GraphQL Yoga).

## 4. Datos

| Tecnología | Por qué |
|---|---|
| PostgreSQL 16+ | ACID, RLS, JSONB para attrs ABAC, logical replication, `SKIP LOCKED` |
| PgBouncer | Pooling bajo ráfagas de SPA |
| Redis | Sesiones, denylist JWT, rate limit, cache de permisos efectivos |
| Kafka | Outbox → proyecciones, integraciones finance/bancos |
| S3-compatible | Comprobantes de pago, exports |

**Migraciones:** Flyway o Goose; nunca cambios manuales en prod.

## 5. Seguridad (herramientas)

| Control | Tooling |
|---|---|
| IdP + MFA | Keycloak (self-host) o Okta/Auth0 (managed) |
| Políticas ABAC | OPA con bundles versionados en Git |
| Escaneo | SAST (Semgrep), SCA (Dependabot/Snyk), container (Trivy) |
| Secrets | Vault + short-lived credentials |
| Threat detection | WAF + anomaly on auth (impossible travel, brute force) |

## 6. Plataforma y HA

```
Internet → CDN (CloudFront/Cloudflare)
        → WAF
        → L7 LB
        → Gateway (×N, multi-AZ)
        → Mesh (mTLS)
        → Workloads HPA/VPA
        → PostgreSQL HA + Redis Cluster + Kafka multi-AZ
```

- **HPA** por CPU/RPS y colas Kafka lag.
- **PodDisruptionBudgets** y topology spread.
- **Chaos engineering** ligero (kill pod / AZ drain) en staging.

## 7. Mapa servicio → stack sugerido

| Servicio | Lenguaje | DB | Notas |
|---|---|---|---|
| Inventory | Go | PostgreSQL | Optimistic locks, alto QPS |
| Catalog | Go | PostgreSQL + Redis | Cache-aside |
| Payroll | Java/Spring | PostgreSQL | Batch + SoD |
| HR / Org | Java o Go | PostgreSQL | Atributos ABAC |
| AuthZ PDP | OPA | — | Sidecar o central |
| Audit | Go | PostgreSQL | Append-only |
| Reporting BFF | Node (Yoga) o Go | Réplicas | GraphQL |
| Notification | Go | — | Consumers Kafka |

## 8. SLOs de referencia

| Indicador | Objetivo |
|---|---|
| Disponibilidad API | 99.9% mensual |
| p95 POST movimiento inventario | < 200 ms |
| p95 cálculo nómina (1k empleados) | < 60 s (batch) |
| RPO / RTO | 5 min / 30 min |
| Error budget auth | < 0.1% fallos 5xx en login |

## 9. Anti-patrones a evitar

- Monolito modular “temporal” sin bounded contexts claros.
- Compartir BD entre microservicios (salvo réplicas de solo lectura explícitas).
- Confiar solo en ocultar botones en la SPA.
- `SELECT … FOR UPDATE` en todos los paths (contención).
- GraphQL mutable sin authz por campo.
- Tokens JWT de larga duración sin rotación ni denylist.
