# 38 — Instalación: bloquear / desbloquear funciones

En el **primer uso** (`/setup`) el dueño elige qué módulos desbloquear. Lo demás queda bloqueado: sin permisos del dueño y oculto en el menú.

## Flujo

```
/setup
  1. Nombre + sucursal
  2. Red (intranet | internet)
  3. Funciones (lock/unlock)
  4. Productos
  5. Dueño
     → POST /setup/complete { network_mode, enabled_modules: [...] }
```

## API

| Método | Ruta | Uso |
|---|---|---|
| `GET /setup/modules` | Catálogo + defaults + `network_modes` |
| `POST /setup/complete` | Acepta `network_mode` + `enabled_modules` |

## Persistencia

- `organizations.enabled_modules` (JSONB array) — migración `031_install_modules.sql`
- `organizations.network_mode` — migración `034_network_mode.sql` (ver [41](./41-network-mode.md))
- `NULL` modules = todo desbloqueado (DEMO / enterprise legacy)
- Array = solo esos códigos activos
- También en `app_install.meta.enabled_modules` / `network_mode`

## Enforcement

1. **Permisos**: el rol `store_owner` solo recibe permisos de módulos activos.
2. **Claims**: enrichment pone `attrs.enabled_modules` y `attrs.network_mode`.
3. **SPA**: `canAccess` exige módulo + permiso; menú usa `NAV_NODES`.
4. **Red**: en intranet se fuerza OFF `storefront` y no hay egress público.

## Catálogo (códigos)

`inventory` (obligatorio), `receiving`, `adjustments`, `pos`, `card_payments`, `customers`, `storefront`, `logistics`, `reports`, `security`, `mail`, `approvals`, `hr`, `dept_managers`, `purchasing`, `facilities`.

Defaults ON: inventario, recepción, caja, clientes, reportes.
