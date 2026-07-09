# 31 — Jefes multi-departamento (sin relación requerida)

## Regla de negocio

Un **jefe** puede ser responsable de **varios departamentos de tienda** a la vez.  
Los departamentos **no tienen que estar relacionados** (ni por categoría, ni por jerarquía de org units).  
Ejemplo válido: el mismo jefe a cargo de **Electrónica** y **Juguetería**.

Esto es independiente del árbol `org_units` (jefes de almacén/región para voids). Ambos modelos coexisten:

| Modelo | Tabla | Claim | Uso |
|---|---|---|---|
| Áreas / almacenes | `org_unit_managers` | `managed_warehouses` | Anular movimientos (ABAC void) |
| Departamentos de tienda | `department_managers` | `managed_departments` | Responsabilidad comercial / catálogo |

## Datos

Migración `024_department_managers.sql`:

```sql
department_managers (
  org_id, department_id → store_departments,
  user_id → users,
  valid_from, valid_to, assigned_by
)
UNIQUE (department_id, user_id)
```

No hay FK entre departamentos asignados: cualquier subconjunto de la sucursal es válido.

## API (gateway)

| Método | Ruta | Permiso |
|---|---|---|
| `GET` | `/org/department-managers?branch_id=` | `store.department.manager.read` |
| `GET` | `/org/department-managers/options?branch_id=` | `store.department.manager.read` |
| `PUT` | `/org/department-managers` | `store.department.manager.assign` |

Body `PUT`:

```json
{
  "user_sub": "usr_dev_wh_manager",
  "branch_id": "br_norte",
  "department_codes": ["electronica", "jugueteria"]
}
```

Reemplaza el conjunto activo del usuario en esa sucursal (cierra los que salen, reactiva/crea los que entran). Tras guardar se invalida el cache de enrichment.

## Claims

El enricher carga `attrs.managed_departments` (códigos) en cada request autenticado.

## UI

Ruta `/settings/jefes` — multi-select de departamentos por persona.  
Roles que asignan: `store_owner`, `regional_manager`, `platform_admin`.

## Demo

Seed: `usr_dev_wh_manager` → Electrónica + Juguetería en `br_norte`.
