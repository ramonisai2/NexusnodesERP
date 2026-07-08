# Diagramas de Arquitectura — NexusnodesERP

## 1. Componentes (C4 Container)

```mermaid
flowchart TB
  subgraph Clients
    SPA["SPA React/TS<br/>Permission-aware UI"]
  end

  subgraph Edge
    CDN["CDN + WAF"]
    LB["Load Balancer L7"]
  end

  subgraph Platform["Kubernetes Cluster (Multi-AZ)"]
    GW["API Gateway / Kong<br/>JWT · Rate limit · Routing"]
    BFF["BFF GraphQL"]
    IAM["identity-service<br/>RBAC+ABAC · MFA"]
    ORG["org-service"]
    INV["inventory-service"]
    PAY["payroll-service"]
    AUD["audit-service"]
    REP["reporting-service"]
    NOTIF["notification-service"]
  end

  subgraph Data
    PG[(PostgreSQL<br/>Primary + Replicas)]
    RD[(Redis Cluster)]
    KF{{Kafka / Redpanda}}
    S3[(Object Storage)]
    OPA[(OPA Policy Bundles)]
  end

  SPA --> CDN --> LB --> GW
  GW --> BFF
  GW --> IAM
  GW --> ORG
  GW --> INV
  GW --> PAY
  BFF --> INV
  BFF --> PAY
  BFF --> ORG
  IAM --> RD
  IAM --> OPA
  IAM --> PG
  ORG --> PG
  INV --> PG
  PAY --> PG
  INV --> KF
  PAY --> KF
  KF --> AUD
  KF --> NOTIF
  KF --> REP
  AUD --> PG
  REP --> PG
  NOTIF --> RD
  PAY --> S3
```

## 2. Flujo de autorización (RBAC + ABAC)

```mermaid
flowchart LR
  A[Request + JWT] --> B{JWT válido?}
  B -->|No| X[401]
  B -->|Sí| C[Extraer claims<br/>roles, branches, amr]
  C --> D[RBAC: ¿rol tiene permiso?]
  D -->|No| Y[403]
  D -->|Sí| E[ABAC: ¿atributos cumplen política?]
  E -->|No| Y
  E -->|Sí| F{¿Requiere step-up MFA?}
  F -->|Sí y amr insuficiente| Z[401 mfa_required]
  F -->|No / OK| G[Forward a microservicio]
  G --> H[PEP local + RLS DB]
  H --> I[200 / 201]
```

## 3. Estados de nómina

```mermaid
stateDiagram-v2
  [*] --> draft
  draft --> calculated: calculate
  calculated --> draft: recalculate
  calculated --> approved: approve + MFA
  approved --> paid: pay + MFA + ABAC limit
  approved --> void: void
  paid --> [*]
  void --> [*]
  draft --> void: cancel
```

## 4. Estados de movimiento de inventario

```mermaid
stateDiagram-v2
  [*] --> draft
  draft --> posted: post + stock update
  draft --> void: cancel
  posted --> void: reverse compensating movement
  posted --> [*]
  void --> [*]
```

## 5. Despliegue HA

```mermaid
flowchart TB
  DNS[DNS / Global Accelerator] --> AZa
  DNS --> AZb
  subgraph AZa[Availability Zone A]
    P1[Pods servicios]
    PG1[(PG Primary)]
  end
  subgraph AZb[Availability Zone B]
    P2[Pods servicios]
    PG2[(PG Sync Standby)]
  end
  P1 -.-> PG1
  P2 -.-> PG1
  PG1 -->|sync replication| PG2
```
