# 23 — Papeletas entre tiendas (papelería de identificación)

## Objetivo

Entre tiendas se usa **papelería** como **papeletas de identificación**: notas que se **imprimen y pegan** en el contenedor, con descripción y **tipo de contenedor**.

Tipos soportados:

| Código | Español |
|---|---|
| `ENVELOPE` | Sobre |
| `BOX` | Caja |
| `PLASTIC_BOX` | Caja plástica |
| `BUNDLE` | Bulto |
| `ORIGINAL_PACK` | Empaque original |

## Modelo

```
shipping_slips
  slip_number (PAP-XXXXXXXX)
  from_branch → to_branch
  container_type + description + contents_summary
  status: DRAFT | PRINTED | IN_TRANSIT | RECEIVED | CANCELLED
  operator_label (asiento compartido)

shipping_slip_lines (opcional)
  sku_code, description, quantity
```

La papeleta **no mueve stock** todavía (el traslado atómico `TRANSFER_*` sigue siendo un slice futuro). Sirve para identificar el bulto físico en tránsito.

## API

| Método | Ruta | Permiso |
|---|---|---|
| `GET /inventory/slips` | listar (`direction=from\|to`) | `inventory.slip.read` |
| `GET /inventory/slips/{id}` | detalle + líneas | `inventory.slip.read` |
| `POST /inventory/slips` | crear borrador | `inventory.slip.create` |
| `POST /inventory/slips/{id}/print` | marcar impresa | `inventory.slip.print` |

## SPA

Menú **Papeletas** → `/inventory/slips`

1. Elige destino, tipo de contenedor y descripción.
2. Crea la papeleta.
3. **Imprimir** abre la hoja lista para pegar y marca `PRINTED`.

## Migración

`infra/postgres/migrations/017_shipping_slips.sql`
