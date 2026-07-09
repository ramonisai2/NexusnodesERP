# 38 — Instalación: bloquear / desbloquear funciones

En el **primer uso** (`/setup`) el dueño elige qué módulos desbloquear. Lo demás queda bloqueado: sin permisos del dueño y oculto en el menú.

## Flujo

```
/setup
  1. Nombre + sucursal
  2. Funciones (lock/unlock)
  3. Productos
  4. Dueño
     → POST /setup/complete { enabled_modules: [...] }
```

## API

| Método | Ruta | Uso |
|---|---|---|
| `GET /setup/modules` | Catálogo + defaults |
| `POST /setup/complete` | Acepta `enabled_modules` |

## Persistencia

- `organizations.enabled_modules` (JSONB array) — migración `031_install_modules.sql`
- `NULL` = todo desbloqueado (DEMO / enterprise legacy)
- Array = solo esos códigos activos
- También en `app_install.meta.enabled_modules`

## Enforcement

1. **Permisos**: el rol `store_owner` solo recibe permisos de módulos activos.
2. **Claims**: enrichment pone `attrs.enabled_modules`.
3. **SPA**: `canAccess` exige módulo + permiso; menú usa `NAV_NODES`.

## Catálogo (códigos)

`inventory` (obligatorio), `receiving`, `adjustments`, `pos`, `customers`, `storefront`, `logistics`, `reports`, `security`, `mail`, `approvals`, `hr`, `dept_managers`, `purchasing`, `facilities`.

Defaults ON: inventario, recepción, caja, clientes, reportes.
