# 34 — Merma / robo y Recursos Humanos

## Merma y robo (ajustes tipificados)

Los movimientos de inventario pueden llevar `reason_code` y `notes`:

| Código | Uso típico | Tipo de movimiento |
|---|---|---|
| `MERMA` | Merma operativa | `ADJUST_OUT` |
| `ROBO` | Robo / extravío | `ADJUST_OUT` |
| `DAMAGE` | Daño físico | `ADJUST_OUT` |
| `EXPIRED` | Caducidad | `ADJUST_OUT` |
| `COUNT_VARIANCE` | Diferencia de conteo | `ADJUST_OUT` o `ADJUST_IN` |
| `FOUND` | Hallazgo / sobrante | `ADJUST_IN` |
| `OTHER` | Otro (con notas) | ambos |

API: `POST /inventory/movements` con `movement_type`, `reason_code`, `notes`.  
Listado: `GET /inventory/movements?reason_code=ROBO`.

SPA: `/inventory/adjustments` — menú **Merma / Robo**.

Permisos: `inventory.adjustment.create` / `inventory.adjustment.read` (también acepta `inventory.movement.create`).

## Recursos Humanos

Apartado `/hr` con pestañas:

1. **Empleados** — `GET /payroll/employees?branch_id=` (sin salarios)
2. **Nómina** — corridas existentes + detalle de líneas (`GET /payroll/runs/{id}`)

Rol nuevo: `hr_officer` (`employee.read`, `employee.write`, lectura/preparación de nómina).

## Migración

`027_merma_robo_hr.sql`

## Fuera de alcance (siguiente)

- Aprobación obligatoria para `ROBO` sobre umbral
- Alta/edición de empleados y contratos
- Conteo cíclico que genere `COUNT_VARIANCE` automáticamente
- Expediente de investigación / evidencia adjunta
