# 27 — Tienda en línea configurable

## Objetivo

Vitrina pública **sin login** para una sucursal, configurable por el dueño:

- Marca, colores, hero y CTA
- Catálogo desde `store_sku_labels` + existencias
- Destacados, precios y badge de stock
- Contacto (teléfono, WhatsApp, email, horario)

No es un carrito/checkout: es catálogo + contacto (ideal para abarrotes / tienda pequeña).

```
/tienda/{slug}  (SPA pública)
  → GET /storefront/public/{slug}  (gateway, sin JWT)
  → inventory GetPublicStorefront
```

Admin autenticado: `/settings/tienda` → `GET/PUT /storefront/settings`.

## Modelo

`branch_storefront_settings` (migración `021_storefront_settings.sql`):

| Campo | Uso |
|---|---|
| `public_slug` | URL `/tienda/{slug}` (único) |
| `published` | Solo vitrinas publicadas son visibles |
| `brand_name`, colores, hero | Branding |
| `show_prices`, `show_stock_badge`, `in_stock_only` | Flags de catálogo |
| `featured_skus` | SKUs destacados |
| contacto / maps | Pie de página |

## Permisos

- `store.storefront.read` — ver configuración
- `store.storefront.manage` — editar y publicar (`store_owner`, `platform_admin`)

## API

| Método | Ruta | Auth |
|---|---|---|
| `GET /storefront/public/{slug}` | vitrina pública | no |
| `GET /storefront/settings?branch_id=` | config | JWT |
| `PUT /storefront/settings` | guardar | JWT + manage |

## SPA

- Pública: `/tienda/:slug`
- Admin: menú **Tienda en línea** → `/settings/tienda`
