# 10 — Jerarquía de jefes y reversión de errores

## Objetivo

Un **jefe de área** (p. ej. almacén) tiene más privilegios que el personal operativo y puede **revertir** movimientos erróneos (“error de dedo” / mala práctica) **solo en su área**.  
El **jefe superior** (regional) puede hacer lo mismo en todas las áreas hijas de su región.

## Modelo

```
Región Norte (REGIONAL_MANAGER: usr_dev_regional)
 └── Área Almacén Norte (AREA_MANAGER: usr_dev_wh_manager)
      └── wh_norte

Región Sur
 └── Área Almacén Sur
      └── wh_sur
```

Tablas:

- `org_units` — árbol de áreas/regiones (`parent_id`)
- `org_unit_managers` — asignación jefe ↔ unidad (`AREA_MANAGER` | `REGIONAL_MANAGER`)
- `warehouses.org_unit_id` — almacén pertenece a un área
- `inventory_movements.reversal_of` / `void_reason` / `voided_by` / `voided_at`

## Permisos

| Rol | Puede |
|---|---|
| `inventory_clerk` | Crear movimientos, ver saldos (no revertir) |
| `warehouse_manager` | Ver/crear + **revertir** en almacenes de su área |
| `regional_manager` | Revertir en almacenes de su región (áreas hijas) |
| `platform_admin` | Todo |

Acción OPA: `inventory.movement.void`  
ABAC: `resource.warehouse_id` ∈ `subject.attrs.managed_warehouses`

## API

- `GET /inventory/movements?branch_id=&warehouse_id=&limit=`
- `POST /inventory/movements/{id}/void`  
  Body: `{ "reason": "Error de captura", "idempotency_key": "..." }`  
  Efecto: marca original `VOID` + inserta movimiento compensatorio `POSTED` enlazado por `reversal_of`.

## Personas de desarrollo

- `persona=wh_manager` — Jefe Almacén Norte  
- `persona=regional` — Jefe Regional Norte (superior)

## Regla de negocio

1. Solo movimientos `POSTED` (no ya anulados) se pueden revertir.
2. La compensación usa cantidad inversa e idempotencia.
3. Queda auditoría (`voided_by`, `void_reason`, outbox `InventoryMovedVoided`).
