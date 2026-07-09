# 28 — Paquetería entre tiendas, CEDI, defectuosos y garantías

## Objetivo

Orquestar envíos físicos entre **tiendas**, **CEDI** y **centros de servicio** (defectuosos, garantías, devoluciones, taller), sin reescribir papeletas / transporte / traslados:

| Pieza | Rol |
|---|---|
| **Papeleta** (`shipping_slips`) | Identificación física del paquete |
| **Hoja de transporte** (`transport_sheets`) | Manifiesto del viaje |
| **Traslado** (`inventory_transfers`) | Verdad de stock (`TRANSFER_OUT` / `TRANSFER_IN`) |
| **Caso de garantía / devolución** | Expediente de negocio que enlaza slip + traslado |

`parcel_kind` clasifica el paquete en los tres documentos:

`TRANSFER` · `CEDI_DISTRIBUTION` · `DEFECTIVE` · `WARRANTY` · `RETURN_TO_CEDI` · `RETURN_TO_VENDOR` · `REPAIR_OUT` · `REPAIR_IN`

## Ciclos de vida

```
Papeleta:   DRAFT → PRINTED → IN_TRANSIT → RECEIVED
                              ↘ CANCELLED

Transporte: DRAFT → PRINTED → IN_TRANSIT → DELIVERED
                              ↘ CANCELLED
  (al salir/entregar, avanza las papeletas enlazadas)

Traslado:   DRAFT → IN_TRANSIT → RECEIVED   (mueve stock)
```

Sucursales: `branch_kind` = `STORE` | `CEDI` | `SERVICE_CENTER`.  
Almacenes: `DEFECTIVE` | `QUARANTINE` | `REPAIR` | `RETURNS` (además de STORE/CEDI/ARRIVAL).

## API

| Método | Ruta | Permiso |
|---|---|---|
| `GET /inventory/parcels` | hub unificado | `inventory.parcel.read` |
| `POST /inventory/slips/{id}/ship\|receive\|cancel` | ciclo papeleta | `inventory.slip.ship/receive/cancel` |
| `POST /inventory/transport-sheets/{id}/depart\|deliver\|cancel` | ciclo transporte | `inventory.transport.depart/deliver/cancel` |
| `GET/POST /inventory/warranty-cases` | garantías | `inventory.warranty.read/create` |
| `GET/POST /inventory/return-cases` | devoluciones | `inventory.return.read/create` |

Query `parcel_kind` en listados de slips / transport / transfers / parcels.

## SPA

Menú **Paquetería** → `/inventory/parcels` (hub + pestañas garantías / devoluciones).  
Papeletas, transporte y traslados exponen `parcel_kind` y botones de ciclo de vida.

## Migración

`infra/postgres/migrations/022_parcel_logistics.sql`

## Relación

- [23 — Papeletas](./23-shipping-slips.md)
- [24 — Transporte](./24-transport-sheets.md)
- [26 — Traslados](./26-inventory-transfers.md)
- [20 — CEDI](./20-cedi-inbound-receiving.md)
