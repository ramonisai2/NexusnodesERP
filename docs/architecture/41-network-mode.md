# 41 — Modo intranet vs salida a internet

## Objetivo

Diferenciar con claridad dos posturas de red en el primer uso:

| Modo | Código | Qué implica |
|---|---|---|
| **Solo intranet** | `intranet` | ERP en LAN/VPN. Sin vitrina pública ni portal de clientes en internet. |
| **Con salida a internet** | `internet` | ERP interno **más** superficies públicas: `/tienda/{slug}`, registro/login de clientes. |

Default de instalación: **`intranet`** (más seguro). DEMO usa **`internet`** para poder recorrer la vitrina.

## Flujo de instalación

```
/setup
  1. Nombre + sucursal
  2. Red (intranet | internet)   ← nuevo
  3. Funciones (módulos)
  4. Productos
  5. Dueño
     → POST /setup/complete { network_mode, enabled_modules }
```

En **intranet**, el módulo `storefront` se fuerza OFF aunque el usuario lo marque.

## Persistencia

- `organizations.network_mode` (`intranet` \| `internet`) — migración `034_network_mode.sql`
- También en `app_install.meta.network_mode`
- Claims: `attrs.network_mode`, `attrs.public_egress` (bool)

## Enforcement

1. **Setup** — `ApplyNetworkModeToModules` quita módulos de salida pública en intranet.
2. **Inventory** — `GetPublicStorefront` / resolución de slug de clientes exigen `network_mode=internet`; publicar vitrina falla en intranet.
3. **Gateway** — `NETWORK_MODE_FORCE=intranet` apaga a nivel proceso las rutas públicas de storefront/clientes (kill switch de despliegue).
4. **SPA** — banner de modo; wizard con dos tarjetas; admin de tienda no deja publicar sin internet.

## Qué NO bloquea intranet

- ERP autenticado (inventario, caja, nómina, correo interno, etc.)
- Subida móvil por QR (`/upload/{token}`) — pensada para teléfonos en la misma LAN
- Setup de primer uso

## Relacionado

- [27 — Tienda en línea](./27-configurable-storefront.md)
- [29 — Cuentas de cliente](./29-customer-accounts-cards.md)
- [38 — Módulos de instalación](./38-install-modules.md)
