# 01 — Arquitectura de Componentes

## 1. Vista lógica (capas)

```
┌─────────────────────────────────────────────────────────────────┐
│  PRESENTACIÓN (SPA)                                             │
│  React/Vue · State global · UI condicionada por claims JWT      │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTPS / WSS
┌────────────────────────────▼────────────────────────────────────┐
│  EDGE / GATEWAY                                                 │
│  CDN · WAF · API Gateway · Rate Limit · mTLS interno            │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│  CAPA DE IDENTIDAD                                              │
│  IdP (OAuth2/OIDC) · MFA · Token Service · Session Store        │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│  MICROSERVICIOS (Stateless)                                     │
│  AuthZ Policy · Inventory · Payroll · HR · Catalog · Audit      │
│  Notification · Reporting (BFF GraphQL)                         │
└───────┬────────────────────┬────────────────────┬───────────────┘
        │                    │                    │
┌───────▼────────┐  ┌────────▼────────┐  ┌────────▼──────────────┐
│  OLTP Primary  │  │  Message Bus    │  │  Caché / Sesiones     │
│  + Réplicas    │  │  (Kafka/NATS)   │  │  Redis Cluster        │
└────────────────┘  └─────────────────┘  └───────────────────────┘
```

## 2. Componentes y responsabilidades

### 2.1 Cliente (SPA)

| Componente | Responsabilidad |
|---|---|
| Shell SPA | Routing, lazy-loading de módulos, layout desktop-like |
| Auth Client | PKCE, refresh silencioso, MFA challenge, almacenamiento seguro de tokens |
| Policy Guard (UI) | Oculta/deshabilita nodos según permisos + atributos (sucursal, turno) |
| State Store | Estado global normalizado (inventario en vista, nómina en edición) |
| Offline Buffer | Cola local de comandos no críticos con sync al recuperar red |

### 2.2 Edge

| Componente | Responsabilidad |
|---|---|
| CDN / Static Host | Assets SPA, cache busting, TLS terminación pública |
| WAF | OWASP Top 10, bot protection, geo/IP rules |
| API Gateway | Enrutamiento, JWT validation, rate limiting, request ID |
| Load Balancer (L7) | Balanceo entre réplicas de gateway y servicios |

### 2.3 Identidad y autorización

| Servicio | Responsabilidad |
|---|---|
| Identity Provider | OAuth2 Authorization Code + PKCE, OIDC, MFA (TOTP/WebAuthn) |
| Token Service | Emisión/rotación de access/refresh tokens (JWT corto + refresh opaco) |
| Policy Decision Point (PDP) | Evalúa RBAC + ABAC; responde allow/deny + obligations |
| Policy Enforcement Point (PEP) | En gateway y en cada microservicio (defense in depth) |

### 2.4 Dominio de negocio

| Microservicio | Bounded Context | Notas |
|---|---|---|
| **Inventory Service** | Stock, almacenes, movimientos, reservas, conteos | Transacciones + optimistic locking |
| **Catalog Service** | Productos, SKUs, UoM, categorías | Lecturas intensivas, cacheable |
| **Payroll Service** | Periodos, conceptos, cálculos, pagos | Transacciones fuertes, idempotencia |
| **HR Service** | Empleados, contratos, centros de costo | Fuente de verdad de atributos ABAC |
| **Org Service** | Organizaciones, sucursales, departamentos | Jerarquía para ABAC |
| **Audit Service** | Append-only log de acciones sensibles | Inmutable, retención legal |
| **Notification Service** | Email/SMS/Push de eventos | Consumidor de eventos |
| **Reporting BFF** | GraphQL federado / agregaciones | Solo lecturas; no escribe OLTP |

### 2.5 Infraestructura de datos

| Store | Uso |
|---|---|
| PostgreSQL Primary | Escrituras OLTP (inventario, nómina) |
| PostgreSQL Read Replicas | Consultas, reportes, GraphQL |
| Redis Cluster | Sesiones, rate-limit, locks distribuidos cortos, cache |
| Kafka / NATS JetStream | Eventos de dominio, outbox, sync asíncrona |
| Object Storage (S3) | Comprobantes, reportes PDF, adjuntos |
| OpenSearch (opcional) | Búsqueda full-text de productos/empleados |

## 3. Alta disponibilidad y tolerancia a fallos

```
                    ┌──────────────┐
   Users ──HTTPS──► │  LB / CDN    │
                    └──────┬───────┘
           ┌───────────────┼───────────────┐
           ▼               ▼               ▼
      Gateway-A       Gateway-B       Gateway-C
           │               │               │
     ┌─────┴───────────────┴───────────────┴─────┐
     │         Service Mesh / mTLS               │
     └─────┬───────────────┬───────────────┬─────┘
           ▼               ▼               ▼
      Inv-Pod×N       Pay-Pod×N       Auth-Pod×N
           │               │               │
           └───────┬───────┴───────┬───────┘
                   ▼               ▼
            PG Primary ◄──sync──► PG Replica(s)
                   │
            Patroni / Cloud HA
```

**Patrones obligatorios:**

- **Health checks** (`/live`, `/ready`) + auto-healing (K8s).
- **Circuit breaker + bulkhead** por dependencia (Resilience4j / Polly / opossum).
- **Timeouts explícitos** y reintentos con jitter solo en operaciones idempotentes.
- **Outbox pattern** para publicar eventos de forma atómica con la transacción de negocio.
- **Idempotency-Key** en POST de movimientos de inventario y corridas de nómina.
- **Multi-AZ** para gateway, servicios y base de datos.

## 4. Comunicación

| Canal | Protocolo | Uso |
|---|---|---|
| Cliente → Gateway | HTTPS REST / GraphQL | Comandos y queries de UI |
| Gateway → Servicios | gRPC o REST interno + mTLS | Baja latencia, tipado |
| Servicio → Servicio | Eventos (async) preferente; sync solo síncrono necesario | Desacoplamiento |
| BFF Reporting | GraphQL | Agregaciones multi-dominio de solo lectura |

## 5. Flujo de request tipificado (inventario)

1. SPA envía `POST /inventory/movements` con `Authorization: Bearer <JWT>` + `Idempotency-Key`.
2. Gateway valida firma JWT, expiración, audience; extrae claims (`sub`, `roles`, `org`, `branch`).
3. PEP consulta PDP: ¿el sujeto puede `inventory.movement.create` en `branch=X` con atributo `warehouse.region`?
4. Inventory Service abre transacción, aplica **optimistic lock** (`version`) o **pessimistic** en cierre de inventario.
5. Escribe movimiento + actualiza stock + inserta outbox event.
6. Commit; consumer publica `InventoryMoved` a Kafka.
7. Proyecciones (reporting, cache) se actualizan de forma asíncrona.
8. Audit Service registra who/what/when/where (IP, device, branch).

Ver diagramas Mermaid en [`../diagrams/`](../diagrams/).
