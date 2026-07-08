# 02 — Seguridad: RBAC + ABAC, JWT, OAuth2 y MFA

## 1. Modelo híbrido RBAC + ABAC

**RBAC** define *qué puede hacer* un rol (permisos base).  
**ABAC** refina *bajo qué condiciones* (sucursal, turno, monto, IP, MFA step-up).

```
Decisión = RBAC(role → permissions) ∧ ABAC(attributes, policy)
```

### Atributos ABAC relevantes

| Sujeto | Recurso | Acción | Entorno |
|--------|---------|--------|---------|
| `user_id`, `roles[]`, `clearance` | `type`, `branch_id`, `owner_id`, `amount` | `create/read/update/delete/approve` | `ip`, `time`, `mfa_level`, `device_trust` |
| `tenant_id`, `department` | `sensitivity`, `period_status` | `export`, `pay` | `geo`, `risk_score` |

### Ejemplo de política (pseudo-Cedar / OPA Rego)

```rego
allow if {
  input.user.roles[_] == "payroll_manager"
  input.action == "payroll.approve"
  input.resource.branch_id == input.user.branch_ids[_]
  input.resource.gross_total <= input.user.approval_limit
  input.env.mfa_level >= 2
}
```

## 2. Identidad y sesión

```
┌────────┐   OIDC    ┌──────────────┐   JWT    ┌────────────┐
│  SPA   │──────────▶│ Auth Server  │─────────▶│  Gateway   │
│        │◀──────────│ (Keycloak /  │◀─────────│  + PDP     │
└────────┘  tokens   │  Auth0/Cognito│  refresh └────────────┘
                     │  + MFA TOTP/  │
                     │  WebAuthn)    │
                     └──────────────┘
```

### Tokens

| Token | TTL | Contenido (claims mínimos) | Almacenamiento cliente |
|-------|-----|----------------------------|------------------------|
| Access JWT | 5–15 min | `sub`, `tid`, `roles`, `branch_ids`, `permissions_hash`, `amr`, `jti` | Memoria (no localStorage) |
| Refresh | 8–24 h / rotativo | `sub`, `sid`, `family_id` | HttpOnly Secure SameSite cookie |
| ID Token | = access | Perfil OIDC | Memoria |

### Requisitos de seguridad JWT

- Algoritmo **RS256/ES256** (nunca HS256 compartido entre servicios).
- `aud` y `iss` validados en gateway y en cada servicio crítico.
- Revocación: denylist Redis por `jti` / `sid` en logout y compromise.
- **Step-up MFA** para acciones sensibles (`payroll.pay`, `stock.adjust > threshold`).
- Binding opcional: DPoP o mTLS token binding en clientes corporativos.

## 3. OAuth2 / OIDC flows

| Cliente | Flow | Notas |
|---------|------|-------|
| SPA pública | Authorization Code + **PKCE** | Sin client secret |
| Integraciones M2M | Client Credentials | Scopes estrechos |
| Empleados internos | OIDC + MFA obligatorio | WebAuthn preferido |

Scopes ejemplo: `inventory.read`, `inventory.write`, `payroll.read`, `payroll.approve`, `audit.read`.

## 4. Enforcement en capas

```
┌──────────────────────────────────────────────────────────┐
│ 1. UI (PEP suave)                                        │
│    Oculta/deshabilita nodos según permission map         │
│    NUNCA es la única defensa                             │
├──────────────────────────────────────────────────────────┤
│ 2. API Gateway (PEP)                                     │
│    JWT válido + rate limit + WAF + ruta permitida        │
├──────────────────────────────────────────────────────────┤
│ 3. Policy Decision Point (identity-service / OPA)        │
│    Evalúa RBAC+ABAC con contexto de request              │
├──────────────────────────────────────────────────────────┤
│ 4. Microservicio (PEP duro)                              │
│    Revalida acción + ownership + branch + version lock   │
├──────────────────────────────────────────────────────────┤
│ 5. Base de datos                                         │
│    Row-Level Security (RLS) por tenant_id / branch_id    │
└──────────────────────────────────────────────────────────┘
```

## 5. Nodos de interfaz dinámicos

La SPA obtiene un **Permission Manifest** post-login:

```json
{
  "version": "2026-07-08.1",
  "nodes": {
    "nav.inventory": { "visible": true },
    "nav.payroll": { "visible": true },
    "inventory.adjust": { "enabled": true, "maxAmount": 10000 },
    "payroll.approve": { "enabled": true, "requiresMfa": true },
    "payroll.pay": { "enabled": false }
  },
  "branches": ["MX-01", "MX-02"],
  "defaultBranch": "MX-01"
}
```

Reglas:

- Cada ruta/componente declara `requiredPermission`.
- Router guarda: sin permiso → 403 UI / redirect.
- Botones y campos se condicionan; montos máximos vienen del manifesto (ABAC).
- El manifesto se refresca en cambio de sucursal o tras step-up MFA.

## 6. Roles iniciales sugeridos

| Rol | Inventario | Nómina | Org |
|-----|------------|--------|-----|
| `super_admin` | Full | Full | Full |
| `branch_manager` | R/W sucursal | Approve sucursal | Read |
| `warehouse_clerk` | Movements, receive | — | — |
| `inventory_auditor` | Read + export | — | — |
| `hr_analyst` | — | Read/prepare | Read employees |
| `payroll_manager` | — | Approve/pay* | — |
| `employee_self` | — | Own payslips | Own profile |
| `auditor` | Read | Read | Read + audit |

\* `pay` exige MFA step-up + límite ABAC.

## 7. Controles de ciberseguridad adicionales

- **mTLS** entre servicios (mesh).
- **Secrets**: rotación automática; nunca en env planos en CI logs.
- **Cifrado**: TLS 1.3 en tránsito; AES-256 en reposo (TDE + campos PII con envelope encryption).
- **PII/nómina**: columnas sensibles cifradas; acceso auditado.
- **Hardening API**: schema validation, max body size, SSRF egress deny-by-default.
- **Supply chain**: SBOM, image signing (Cosign), dependency scanning.
- **SIEM**: envío de auth failures, privilege escalations, bulk exports.
- **Backup**: cifrado, tested restores, acceso segregado.
- **Cumplimiento**: retención de audit trail ≥ 7 años (configurable por jurisdicción laboral).
