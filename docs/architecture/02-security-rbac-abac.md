# 02 — Seguridad: RBAC + ABAC, JWT, OAuth2/MFA

## 1. Modelo de amenaza (resumen)

| Amenaza | Mitigación |
|---|---|
| Credenciales robadas | MFA (TOTP/WebAuthn), refresh rotativo, detección de anomalías |
| Privilege escalation | Deny-all, PDP central, least privilege, revisión de roles |
| IDOR / cross-branch | ABAC por `org_id`/`branch_id` en TODA query |
| Token replay | JWT corto (5–15 min), `jti` en denylist Redis, binding de device |
| Injection / XSS | Prepared statements, CSP, `packages/go/secure` + SPA sanitize (ver [30](./30-security-hardening.md)), GraphQL depth limits |
| Insider abuse | Audit append-only, SoD (segregation of duties) en nómina |

## 2. Autenticación (AuthN)

### Flujo OAuth2 Authorization Code + PKCE + MFA

```
SPA                IdP                 MFA              Token Svc
 │                  │                   │                  │
 │── Auth request ─►│                   │                  │
 │   (PKCE challenge)                   │                  │
 │◄─ Login UI ──────│                   │                  │
 │── Credentials ──►│                   │                  │
 │                  │── Challenge ─────►│                  │
 │◄─────────────────│◄── OTP/WebAuthn ──│                  │
 │── Auth code ────►│─────────────────────────────────────►│
 │◄─ Access JWT + Refresh (httpOnly cookie / secure store)─┤
```

**Tokens:**

| Tipo | Formato | TTL | Contenido |
|---|---|---|---|
| Access | JWT firmado (RS256/ES256) | 5–15 min | `sub`, `roles`, `permissions` (compactas), `org_id`, `branch_ids`, `amr`, `sid` |
| Refresh | Opaco (referencia en Redis/BD) | 8–24 h / rotativo | Solo `sid` + device fingerprint hash |
| ID Token | OIDC JWT | = access | Perfil mínimo para UI |

**Claims mínimos del Access Token:**

```json
{
  "sub": "usr_01H...",
  "org_id": "org_01H...",
  "branch_ids": ["br_norte", "br_sur"],
  "roles": ["inventory_manager", "payroll_viewer"],
  "permissions": ["inventory.movement.create", "payroll.run.read"],
  "attrs": {
    "cost_center_ids": ["cc_100"],
    "max_payroll_amount": 500000
  },
  "amr": ["pwd", "otp"],
  "sid": "sess_...",
  "jti": "tok_...",
  "exp": 1710000000
}
```

## 3. Autorización (AuthZ): RBAC + ABAC

### 3.1 RBAC — qué puede hacer el rol

```
User ──N:M── Role ──N:M── Permission
                 │
                 └── Scope (module:action:resource)
```

**Roles semilla (ejemplo):**

| Rol | Permisos clave |
|---|---|
| `platform_admin` | Gestión global de org/roles (break-glass auditado) |
| `branch_manager` | Lectura amplia de sucursal; aprueba ajustes de inventario |
| `inventory_clerk` | Movimientos, recepciones, traslados en su almacén |
| `inventory_auditor` | Solo lectura + conteos cíclicos |
| `hr_officer` | Empleados, contratos |
| `payroll_analyst` | Preparar corrida de nómina |
| `payroll_approver` | Aprobar/pagar (SoD: no puede ser quien prepara) |
| `finance_viewer` | Reportes financieros de solo lectura |
| `warehouse_clerk` / `cedi_clerk` / `dispatch_clerk` | Operación de almacén, CEDI y reparto (ver [36](./36-org-profiles.md)) |
| `sales_associate` / `cashier` | Piso de ventas y cobro |
| `warranty_clerk` / `ecommerce_clerk` | Garantías y ventas en línea |
| `store_coordinator` / `store_admin` / `area_manager` | Coordinación y jefatura de tienda/área |
| `purchasing_clerk` / `purchasing_manager` | Órdenes de compra |
| `facilities_staff` | Limpieza / mantenimiento (work orders) |
| `security_officer` | Vigilancia / sellos (ver [35](./35-security-officer-seals.md)) |
| `webmaster` | Reportes impresos/digitales de dominios administrados (ver [37](./37-webmaster-reports.md)) |

### 3.2 ABAC — bajo qué condiciones

El PDP evalúa **atributos del sujeto, recurso, acción y entorno**:

| Dimensión | Ejemplos de atributos |
|---|---|
| Sujeto | `roles`, `branch_ids`, `cost_center_ids`, `clearance`, `mfa_level` |
| Recurso | `resource.branch_id`, `warehouse_id`, `employee.branch_id`, `amount` |
| Acción | `inventory.movement.create`, `payroll.run.approve` |
| Entorno | `time_of_day`, `ip_range`, `device_trust`, `channel` |

**Ejemplo de política (pseudo-Cedar/Rego):**

```
permit(
  principal,
  action == "inventory.movement.create",
  resource
) when {
  principal.roles.contains("inventory_clerk")
  && principal.branch_ids.contains(resource.branch_id)
  && context.mfa_level >= 1
  && resource.quantity.abs() <= principal.attrs.max_adjustment
};
```

```
permit(
  principal,
  action == "payroll.run.approve",
  resource
) when {
  principal.roles.contains("payroll_approver")
  && principal.org_id == resource.org_id
  && principal.sub != resource.prepared_by   // Segregation of Duties
  && resource.total_amount <= principal.attrs.max_payroll_amount
};
```

### 3.3 Enforcement en capas

| Capa | Mecanismo |
|---|---|
| SPA (UX) | `PolicyGuard`: no renderiza rutas/botones sin permiso (no es seguridad real) |
| API Gateway (PEP) | Valida JWT + permisos gruesos + rate limit |
| Microservicio (PEP) | Re-evalúa con PDP; filtra por `org_id`/`branch_id` en repositorio |
| Base de datos | Row-Level Security (PostgreSQL RLS) como última línea |

## 4. Condicionamiento dinámico de nodos UI

Cada nodo del menú/módulo declara metadatos de política:

```ts
{
  id: "nav.inventory.adjustments",
  route: "/inventory/adjustments",
  require: {
    permissions: ["inventory.adjustment.create"],
    anyBranch: true,
    minAmr: ["pwd", "otp"]
  }
}
```

Al hidratar la sesión, el cliente:

1. Lee claims del JWT (+ endpoint `/me/effective-permissions` para permisos finos).
2. Evalúa el árbol de navegación y formularios.
3. Suscribe a cambios de sucursal activa → re-filtra nodos y queries GraphQL con variable `branchId`.

## 5. Controles adicionales

- **mTLS** entre servicios en el mesh.
- **Secrets** en vault (no en env planos en prod).
- **Rotación de claves JWT** (JWKS) con overlap.
- **Denylist** de `jti`/`sid` en logout y detección de robo.
- **CSP + Trusted Types** en SPA.
- **PII encryption** at-rest para datos sensibles de nómina (salario, cuentas bancarias) con envelope encryption.
- **Retention & legal hold** en Audit y Payroll artifacts.
