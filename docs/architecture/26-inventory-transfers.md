# 26 — Traslados de stock (TRANSFER_OUT / TRANSFER_IN)

## Objetivo

Mover mercancía **de verdad** entre almacenes/sucursales con ciclo:

```
DRAFT → IN_TRANSIT → RECEIVED
         ↘ CANCELLED
```

- **Embarcar** (`ship`): baja stock en origen con `TRANSFER_OUT` (una TX).
- **Recibir** (`receive`): sube stock en destino con `TRANSFER_IN`.
- **Cancelar** en tránsito: devuelve stock al origen.

Las **papeletas** y **hojas de transporte** siguen siendo papelería; un traslado puede enlazar papeletas y actualizar su estado al embarcar/recibir.

## Modelo

| Tabla | Rol |
|---|---|
| `inventory_transfers` | Cabecera `TRF-…` |
| `inventory_transfer_lines` | SKU + cantidad + ids de movimientos |
| `inventory_transfer_slip_links` | Papeletas opcionales |

`inventory_movements.source_transfer_id` / `transfer_line_id` enlazan los movimientos.

## API

| Método | Ruta | Permiso |
|---|---|---|
| `GET /inventory/transfers` | listar | `inventory.transfer.read` |
| `GET /inventory/transfers/{id}` | detalle | `inventory.transfer.read` |
| `POST /inventory/transfers` | crear borrador | `inventory.transfer.create` |
| `POST /inventory/transfers/{id}/ship` | embarcar | `inventory.transfer.ship` |
| `POST /inventory/transfers/{id}/receive` | recibir | `inventory.transfer.receive` |
| `POST /inventory/transfers/{id}/cancel` | cancelar | `inventory.transfer.cancel` |

Query `direction=from|to|all` + `branch_id` para salientes/entrantes.

## SPA

Menú **Traslados** → `/inventory/transfers`

## Migración

`infra/postgres/migrations/020_inventory_transfers.sql`

## Relación

- [20 — CEDI](./20-cedi-inbound-receiving.md): recepción de proveedor ≠ traslado entre tiendas
- [23 — Papeletas](./23-shipping-slips.md): identificación; no mueven stock solas
- [24 — Transporte](./24-transport-sheets.md): manifiesto de viaje
