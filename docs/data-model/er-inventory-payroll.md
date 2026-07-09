# Modelo Entidad–Relación Inicial

> Módulos: **Seguridad (RBAC/ABAC)**, **Inventario**, **Nómina**.  
> Convenciones: `uuid` PK, `org_id` en todas las tablas de negocio, `version` para optimistic locking, `created_at`/`updated_at`, soft-delete solo donde no haya requisitos legales de inmutabilidad.

---

## Diagrama ER (Mermaid)

```mermaid
erDiagram
  ORGANIZATIONS ||--o{ BRANCHES : has
  ORGANIZATIONS ||--o{ USERS : employs
  BRANCHES ||--o{ WAREHOUSES : has
  BRANCHES ||--o{ DEPARTMENTS : has

  USERS ||--o{ USER_ROLES : assigned
  ROLES ||--o{ USER_ROLES : granted
  ROLES ||--o{ ROLE_PERMISSIONS : includes
  PERMISSIONS ||--o{ ROLE_PERMISSIONS : mapped
  USERS ||--o{ USER_ATTRIBUTES : has
  POLICIES ||--o{ POLICY_BINDINGS : bound

  USERS ||--o{ SESSIONS : opens
  SESSIONS ||--o{ TOKEN_DENYLIST : may_revoke

  PRODUCTS ||--o{ PRODUCT_SKUS : variants
  PRODUCT_SKUS ||--o{ STOCK_BALANCES : tracked_as
  WAREHOUSES ||--o{ STOCK_BALANCES : holds
  WAREHOUSES ||--o{ INVENTORY_MOVEMENTS : source_or_dest
  PRODUCT_SKUS ||--o{ INVENTORY_MOVEMENTS : moved
  USERS ||--o{ INVENTORY_MOVEMENTS : posted_by
  STOCK_BALANCES ||--o{ STOCK_RESERVATIONS : reserves

  BRANCHES ||--o{ EMPLOYEES : staffs
  EMPLOYEES ||--o{ EMPLOYMENT_CONTRACTS : has
  EMPLOYEES ||--o{ PAYROLL_LINES : paid_as
  PAYROLL_PERIODS ||--o{ PAYROLL_RUNS : contains
  PAYROLL_RUNS ||--o{ PAYROLL_LINES : details
  PAYROLL_CONCEPTS ||--o{ PAYROLL_LINES : applies
  PAYROLL_RUNS ||--o{ PAYROLL_APPROVALS : audited_by
  USERS ||--o{ PAYROLL_APPROVALS : actor

  ORGANIZATIONS {
    uuid id PK
    string code UK
    string name
    string status
  }

  BRANCHES {
    uuid id PK
    uuid org_id FK
    string code
    string name
    string region
    boolean active
  }

  USERS {
    uuid id PK
    uuid org_id FK
    string email UK
    string status
    string mfa_methods
  }

  ROLES {
    uuid id PK
    uuid org_id FK
    string code UK
    string name
  }

  PERMISSIONS {
    uuid id PK
    string code UK
    string module
    string action
    string resource
  }

  USER_ROLES {
    uuid user_id FK
    uuid role_id FK
    uuid branch_id FK
    timestamptz valid_from
    timestamptz valid_to
  }

  USER_ATTRIBUTES {
    uuid user_id FK
    string attr_key
    jsonb attr_value
  }

  POLICIES {
    uuid id PK
    string name
    string engine
    text policy_body
    int version
  }

  WAREHOUSES {
    uuid id PK
    uuid branch_id FK
    string code
    boolean allow_negative
  }

  PRODUCTS {
    uuid id PK
    uuid org_id FK
    string sku_base
    string name
  }

  PRODUCT_SKUS {
    uuid id PK
    uuid product_id FK
    string sku UK
    string uom
  }

  STOCK_BALANCES {
    uuid id PK
    uuid warehouse_id FK
    uuid sku_id FK
    numeric on_hand
    numeric reserved
    int version
  }

  INVENTORY_MOVEMENTS {
    uuid id PK
    uuid org_id FK
    uuid branch_id FK
    uuid sku_id FK
    uuid warehouse_id FK
    string movement_type
    numeric quantity
    string status
    uuid posted_by FK
    string idempotency_key UK
    int version
  }

  STOCK_RESERVATIONS {
    uuid id PK
    uuid stock_balance_id FK
    numeric quantity
    string status
    timestamptz expires_at
  }

  EMPLOYEES {
    uuid id PK
    uuid org_id FK
    uuid branch_id FK
    uuid user_id FK
    string employee_number
    string status
  }

  EMPLOYMENT_CONTRACTS {
    uuid id PK
    uuid employee_id FK
    string contract_type
    numeric base_salary_encrypted
    uuid cost_center_id
    date start_date
    date end_date
  }

  PAYROLL_PERIODS {
    uuid id PK
    uuid org_id FK
    date start_date
    date end_date
    string status
  }

  PAYROLL_RUNS {
    uuid id PK
    uuid period_id FK
    uuid branch_id FK
    string status
    uuid prepared_by FK
    numeric total_amount
    int version
  }

  PAYROLL_CONCEPTS {
    uuid id PK
    uuid org_id FK
    string code
    string concept_type
    string calc_rule
  }

  PAYROLL_LINES {
    uuid id PK
    uuid run_id FK
    uuid employee_id FK
    uuid concept_id FK
    numeric amount
    string idempotency_key UK
  }

  PAYROLL_APPROVALS {
    uuid id PK
    uuid run_id FK
    uuid actor_user_id FK
    string action
    timestamptz at
    jsonb metadata
  }

  SESSIONS {
    uuid id PK
    uuid user_id FK
    string device_hash
    timestamptz expires_at
  }

  AUDIT_LOG {
    uuid id PK
    uuid org_id
    uuid actor_user_id
    string action
    string resource_type
    uuid resource_id
    jsonb before_after
    timestamptz at
  }
```

---

## 1. Dominio de seguridad y organización

| Entidad | Propósito |
|---|---|
| `organizations` | Tenant lógico |
| `branches` | Sucursales; atributo ABAC clave |
| `departments` | Centros organizativos / cost centers |
| `users` | Identidades de aplicación (ligadas al IdP vía `idp_sub`) |
| `roles` / `permissions` / `user_roles` | RBAC; `user_roles.branch_id` permite rol scoped a sucursal |
| `user_attributes` | Atributos ABAC (`max_payroll_amount`, `warehouse_ids`, etc.) |
| `policies` / `policy_bindings` | Documentos OPA/Cedar versionados |
| `sessions` / `token_denylist` | Refresh opacos + revocación `jti` |
| `audit_log` | Append-only de acciones sensibles |

**Índices críticos:** `(org_id, email)`, `(user_id, role_id, branch_id)`, GIN sobre `user_attributes.attr_value`.

## 2. Dominio de inventario

| Entidad | Propósito |
|---|---|
| `products` / `product_skus` | Catálogo y unidad transaccional (SKU) |
| `warehouses` | Ubicaciones de stock por sucursal |
| `stock_balances` | `on_hand`, `reserved`, `version` (optimistic lock) |
| `inventory_movements` | Hecho inmutable de cambio (`RECEIPT`, `ISSUE`, `ADJUST`, `TRANSFER_*`, `VOID`) |
| `stock_reservations` | Soft-allocation con expiración |
| `inventory_counts` / `inventory_count_lines` *(fase 2)* | Conteos cíclicos con lock pesimista |

**Constraints:**

- `UNIQUE (warehouse_id, sku_id)` en `stock_balances`.
- `UNIQUE (org_id, idempotency_key)` en movimientos.
- Check: `on_hand >= 0` si `warehouses.allow_negative = false`.
- RLS: `org_id = current_setting('app.org_id')`.

## 3. Dominio de nómina

| Entidad | Propósito |
|---|---|
| `employees` | Trabajadores; `branch_id` alimenta ABAC |
| `employment_contracts` | Salario base cifrado, vigencia, cost center |
| `payroll_periods` | Ventana temporal (quincena/mes) |
| `payroll_runs` | Corrida por sucursal/periodo; máquina de estados |
| `payroll_concepts` | Percepciones/deducciones y regla de cálculo |
| `payroll_lines` | Resultado por empleado/concepto (idempotente) |
| `payroll_approvals` | Trail SoD (prepare ≠ approve) |
| `payroll_adjustments` *(fase 2)* | Correcciones post-cierre en periodo nuevo |
| `pay_disbursements` *(fase 2)* | Pagos bancarios / recibos en S3 |

**Constraints:**

- `CHECK (prepared_by <> approved_by)` enforced en servicio + trigger.
- Inmutabilidad post-`CLOSED` vía trigger `prevent_mutation`.
- Cifrado de `base_salary_encrypted` y cuentas bancarias (envelope + KMS).

## 4. Matriz de acceso (recurso × rol) — semilla

| Recurso / Acción | clerk | inv_auditor | branch_mgr | pay_analyst | pay_approver |
|---|---|---|---|---|---|
| `inventory.movement.create` | ✓ (su branch) | | ✓ | | |
| `inventory.balance.read` | ✓ | ✓ | ✓ | | |
| `inventory.count.close` | | ✓ | ✓ | | |
| `payroll.run.prepare` | | | | ✓ | |
| `payroll.run.approve` | | | | | ✓ (≠ preparador) |
| `payroll.run.read` | | | ✓ | ✓ | ✓ |
| `employee.salary.read` | | | | ✓* | ✓* |

\* Requiere MFA step-up + audit obligatorio.

## 5. SQL — esqueleto de tablas núcleo

```sql
-- Seguridad (extracto)
CREATE TABLE organizations (
  id UUID PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE'
);

CREATE TABLE branches (
  id UUID PRIMARY KEY,
  org_id UUID NOT NULL REFERENCES organizations(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  region TEXT,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  UNIQUE (org_id, code)
);

CREATE TABLE roles (
  id UUID PRIMARY KEY,
  org_id UUID NOT NULL REFERENCES organizations(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE (org_id, code)
);

CREATE TABLE permissions (
  id UUID PRIMARY KEY,
  code TEXT NOT NULL UNIQUE, -- e.g. inventory.movement.create
  module TEXT NOT NULL,
  action TEXT NOT NULL,
  resource TEXT NOT NULL
);

CREATE TABLE user_roles (
  user_id UUID NOT NULL,
  role_id UUID NOT NULL,
  branch_id UUID REFERENCES branches(id), -- NULL = all branches in org
  valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
  valid_to TIMESTAMPTZ,
  PRIMARY KEY (user_id, role_id, COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'))
);

-- Inventario (extracto)
CREATE TABLE stock_balances (
  id UUID PRIMARY KEY,
  warehouse_id UUID NOT NULL REFERENCES warehouses(id),
  sku_id UUID NOT NULL REFERENCES product_skus(id),
  on_hand NUMERIC(18,4) NOT NULL DEFAULT 0,
  reserved NUMERIC(18,4) NOT NULL DEFAULT 0,
  version INT NOT NULL DEFAULT 1,
  UNIQUE (warehouse_id, sku_id)
);

CREATE TABLE inventory_movements (
  id UUID PRIMARY KEY,
  org_id UUID NOT NULL,
  branch_id UUID NOT NULL,
  sku_id UUID NOT NULL,
  warehouse_id UUID NOT NULL,
  movement_type TEXT NOT NULL,
  quantity NUMERIC(18,4) NOT NULL,
  status TEXT NOT NULL,
  posted_by UUID NOT NULL,
  idempotency_key TEXT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, idempotency_key)
);

-- Nómina (extracto)
CREATE TABLE payroll_runs (
  id UUID PRIMARY KEY,
  period_id UUID NOT NULL REFERENCES payroll_periods(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  status TEXT NOT NULL,
  prepared_by UUID NOT NULL,
  total_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
  version INT NOT NULL DEFAULT 1
);

CREATE TABLE payroll_approvals (
  id UUID PRIMARY KEY,
  run_id UUID NOT NULL REFERENCES payroll_runs(id),
  actor_user_id UUID NOT NULL,
  action TEXT NOT NULL, -- PREPARE|APPROVE|PAY|CLOSE|REJECT
  at TIMESTAMPTZ NOT NULL DEFAULT now(),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);
```

## 6. Decisiones de modelado

1. **Tenant en fila (`org_id`)** + RLS: aislamiento fuerte sin schema-per-tenant inicial.
2. **Rol scoped a sucursal** en `user_roles.branch_id`: combina RBAC clásico con frontera ABAC.
3. **Movimientos como fuente de verdad** del inventario; el balance es proyección transaccional.
4. **Nómina inmutable al cierre**: evita fraude y simplifica auditoría forense.
5. **Idempotency keys** en movimientos y líneas: seguros ante retries de SPA/gateway.
6. **Atributos ABAC en JSONB** versionables sin migrar esquema por cada política nueva.
