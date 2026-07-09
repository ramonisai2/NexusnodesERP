# 11 — Departamentos de tienda y artículos multi-área

## Objetivo

En una tienda, los artículos se organizan por **departamento** y **categoría**.  
Un mismo artículo puede aparecer en **varios** departamentos según la necesidad de la tienda  
(p. ej. un coleccionable en **Electrónica → Videojuegos** y también en **Juguetería → Coleccionables**).

## Modelo

```
Sucursal (tienda)
 └── Departamento: Electrónica
 │    ├── Cómputo
 │    ├── Video
 │    ├── Telefonía
 │    └── Videojuegos  ← figura coleccionable
 └── Departamento: Juguetería
      ├── Coleccionables  ← misma figura coleccionable
      └── Juegos de mesa
```

Tablas:

| Tabla | Rol |
|---|---|
| `store_departments` | Departamentos por sucursal (`branch_id`) |
| `department_categories` | Categorías (árbol opcional con `parent_id`) dentro del depto |
| `product_placements` | N:N producto ↔ departamento/categoría (`is_primary`) |

El **stock** sigue siendo único por almacén+SKU; la colocación es catálogo/merchandising, no inventarios duplicados.

## Jefes de departamento

Un jefe puede responsabilizarse de **varios** departamentos aunque no estén relacionados.  
Ver [31 — Jefes multi-departamento](./31-department-managers.md) (`department_managers` + `managed_departments`).

## API

- `GET /inventory/departments?branch_id=` — árbol depto → categorías
- `GET /inventory/balances?department=&category=` — existencias filtradas por colocación
- `GET /inventory/catalog?branch_id=&department=&category=` — artículos con sus ubicaciones

## Demo

- `LAPTOP-14` → Electrónica / Cómputo  
- `PHONE-X` → Electrónica / Telefonía  
- `FIG-COL-01` → Electrónica / Videojuegos **y** Juguetería / Coleccionables  
