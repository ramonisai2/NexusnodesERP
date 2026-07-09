# 36 — Matriz de perfiles operativos (privilegios)

Perfiles de usuario para la operación diaria de tiendas, CEDI, centros de reparto, ventas, compras, RR.HH., seguridad e instalaciones. Cada perfil es un **rol** con permisos de menor privilegio; el enrichment de claims desde BD aplica los grants reales.

## Migración

`infra/postgres/migrations/029_org_profiles.sql`

- Permisos nuevos: `purchasing.order.{read,create,approve}`, `facilities.workorder.{read,create,close}`
- Roles nuevos + `role_permissions` + usuarios demo `usr_dev_*`

## Personas demo (`POST /auth/dev-token?persona=…`)

| Persona | Rol | Enfoque |
|---|---|---|
| `warehouse` | `warehouse_clerk` | Empleado de almacén |
| `cedi` | `cedi_clerk` | Empleado de CEDI / centro de distribución |
| `dispatch` | `dispatch_clerk` | Empleado de centro de reparto |
| `sales` | `sales_associate` | Vendedor de piso |
| `cashier` | `cashier` | Cobrador / caja |
| `warranty` | `warranty_clerk` | Garantías |
| `ecommerce` | `ecommerce_clerk` | Ventas en línea |
| `coordinator` | `store_coordinator` | Coordinador de tienda |
| `store_admin` | `store_admin` | Administrador de tienda |
| `area` | `area_manager` | Jefe de área |
| `purchasing` | `purchasing_clerk` | Compras (analista) |
| `purchasing_mgr` | `purchasing_manager` | Compras (gerente) |
| `facilities` | `facilities_staff` | Limpieza / mantenimiento |
| `hr` | `hr_officer` | Recursos Humanos |
| `security` | `security_officer` | Seguridad / vigilancia (ver [35](./35-security-officer-seals.md)) |
| `wh_manager` | `warehouse_manager` | Jefe de almacén (jerarquía) |
| `regional` | `regional_manager` | Gerente regional |
| `owner` | `store_owner` | Dueño de tienda pequeña |
| `analyst` / `approver` | nómina | SoD de nómina |

## Matriz de privilegios (resumen)

| Perfil | Inventario | Caja / POS | Logística | Seguridad | Compras | Instalaciones | RR.HH. |
|---|---|---|---|---|---|---|---|
| Almacén | R/W + merma | — | slips / transfers | — | — | — | — |
| CEDI | R/W + inbound camión | — | transport + shipment | sellos / reporte | — | — | — |
| Reparto | lectura + ship/receive | — | transport / slips | sellos | — | — | — |
| Vendedor | lectura | venta | — | — | — | — | — |
| Cobrador | lectura | venta + void | — | — | — | — | — |
| Garantías | warranty / returns | — | slips / transfer | — | — | — | — |
| E-commerce | lectura | lectura ventas | slips / storefront | — | — | — | — |
| Coordinador | R/W + merma | venta + void | slips / transfers | — | — | WO create | — |
| Admin tienda | amplio + void | POS + settings | amplio | sellos | lectura OC | WO full | empleados lectura |
| Jefe de área | amplio + void | venta | amplio | sellos | — | — | — |
| Compras | lectura / receipts | — | shipment lectura | — | OC create | — | — |
| Gerente compras | receipts | — | shipment | — | OC approve | — | — |
| Limpieza / mant. | balance lectura | — | — | — | — | WO full | — |
| RR.HH. | — | — | — | — | — | — | empleados + nómina lectura |
| Seguridad | lectura logística | — | transport / slips | reporte + sellos | — | — | — |
| Regional / dueño | amplio | según rol | amplio | según rol | OC full* | WO full* | según rol |

\* `regional_manager`, `store_owner` y `platform_admin` reciben grants de compras e instalaciones en la migración 029.

## SPA

Login → «perfiles avanzados» → selector de perfil operativo (evita 20 botones).

## Notas

- **Compras** e **instalaciones** son stubs de privilegio (permisos + OPA listos); pantallas CRUD completas pueden llegar después.
- Sin usuario en BD (`idp_sub`), el enrichment deja el token sin permisos → 403. Los `usr_dev_*` de 029 cubren las personas nuevas.
- OPA: acciones `purchasing.order.*` y `facilities.workorder.*` en `infra/opa/policies/authz.rego`.
