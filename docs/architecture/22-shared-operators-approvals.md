# 22 — Operadores compartidos y supervisión de jefes

## Objetivo

Varias personas pueden **compartir el mismo nombre de usuario** (cuenta de caja / almacén) y trabajar **en paralelo**. Cada una se identifica con un **nombre de operador** y, opcionalmente, una **estación**. Las acciones críticas (p. ej. anular un movimiento) pasan por una **cola de aprobación** que decide un jefe.

## Modelo

```
Cuenta compartida: usr_dev_analyst (inventory_clerk)
 ├── Estación Ana · POS-01  → session A (operator_label=Ana)
 ├── Estación Luis · POS-02 → session B (operator_label=Luis)
 └── …
Acciones críticas → approval_requests (PENDING)
 └── Jefe (warehouse_manager / regional) → APPROVED | REJECTED
      └── si APPROVED: ejecuta inventory.movement.void
```

Tablas / columnas:

- `sessions.operator_label`, `station_id`, `jti`, `last_seen_at`, `org_id` — asientos concurrentes bajo el mismo `user_id`
- `inventory_movements.operator_label`, `session_id` — atribución del operador (no solo la cuenta)
- `approval_requests` — cola maker-checker (PENDING / APPROVED / REJECTED / …)

## Permisos

| Código | Quién | Para qué |
|---|---|---|
| `session.operator` | clerk, managers, owner | Abrir asiento de operador |
| `inventory.movement.void.request` | clerk, store_owner | Pedir anulación (no ejecutar) |
| `approval.read` | clerk + managers | Ver cola |
| `approval.decide` | warehouse_manager, regional, payroll_approver | Aprobar / rechazar |
| `inventory.movement.void` | managers (ABAC almacén) | Ejecutar anulación directa o tras aprobar |

## API (gateway)

| Método | Ruta | Notas |
|---|---|---|
| `POST` | `/auth/dev-token?persona=&operator_label=&station_id=` | Emite JWT + abre estación |
| `POST` | `/auth/operator-signin` | Re-emite token con operador (sesión autenticada) |
| `GET` | `/auth/stations` | Estaciones activas del mismo `sub` |
| `DELETE` | `/auth/stations/{id}` | Cierra asiento |
| `POST` | `/approvals` | Crea solicitud (void, …) |
| `GET` | `/approvals` | Pendientes |
| `POST` | `/approvals/{id}/decide` | `{ "approve": true\|false, "reason": "…" }` — si aprueba void, llama a inventory |

Headers propagados al backend: `X-Operator-Label`, `X-Session-Id`.

## Flujo crítico (void)

1. Operador (clerk) en Inventario → **Pedir al jefe** → `POST /approvals`.
2. Jefe en **Aprobaciones** → Aprobar → gateway valida `inventory.movement.void` (almacén gestionado) y ejecuta void.
3. Rechazo solo cierra la solicitud; el movimiento permanece `POSTED`.

## SPA

- Login: campos “Tu nombre en el puesto” + estación opcional.
- Topbar: badge con el operador activo.
- Inventario: clerks ven **Pedir al jefe**; managers siguen con **Revertir** directo.
- Navegación **Aprobaciones** (`/approvals`).

## Migración

`infra/postgres/migrations/016_shared_operators_approvals.sql`
