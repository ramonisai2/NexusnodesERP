# 20 — CEDI, recepción y almacén de llegada

## Objetivo

Gestionar **centros de distribución (CEDI)** y la **recepción de mercancía** con captura de factura, entrada de stock y etiquetas de recepción. Para tienda pequeña, un solo **almacén de llegada** (`ARRIVAL`) cubre el mismo flujo sin complejidad de CEDI.

## Modelo de almacenes

| `warehouse_kind` | Uso |
|---|---|
| `STORE` | Existencias de piso / sucursal |
| `CEDI` | Centro de distribución (hub) |
| `ARRIVAL` | Bahía única de llegada (tienda pequeña / abarrotes) |

Demo enterprise: sucursal `br_cedi` + almacén `cedi_centro` (`CEDI`).  
Wizard abarrotes: almacén `principal` marcado como `ARRIVAL`.

## Flujo de recepción

```
SPA /inventory/receiving
  → POST /inventory/receipts          (DRAFT + líneas + factura)
  → POST /inventory/receipts/{id}/post
       1. upsert stock_balances si el SKU es nuevo en ese almacén
       2. movimiento RECEIPT por línea (source_receipt_id)
       3. upsert store_sku_labels (texto/precio de recepción)
       4. outbox InboundReceiptPosted
```

## API

| Método | Ruta | Permiso |
|---|---|---|
| `GET /inventory/warehouses` | lista por sucursal / kind | `inventory.warehouse.read` |
| `GET /inventory/receipts` | historial | `inventory.receipt.read` |
| `GET /inventory/receipts/{id}` | detalle + etiquetas si POSTED | `inventory.receipt.read` |
| `POST /inventory/receipts` | crear borrador (factura + líneas) | `inventory.receipt.create` |
| `POST /inventory/receipts/{id}/post` | confirmar llegada | `inventory.receipt.post` |

Campos de factura: `supplier_name`, `invoice_number`, `invoice_date`, `notes`, `unit_cost` por línea.

## Tienda pequeña vs CEDI

| Escenario | Destino por defecto | Notas |
|---|---|---|
| Abarrotes / dueño | `principal` (`ARRIVAL`) | Un solo punto de llegada |
| Empresa con CEDI | `cedi_centro` (`CEDI`) | Luego se puede trasladar a tiendas (`TRANSFER_*`, slice futuro) |

## Arranque

```bash
# aplicar 015_cedi_inbound_receipts.sql
make migrate   # o psql -f infra/postgres/migrations/015_...
make run-inventory
make run-gateway
```

SPA: menú **Recepción**.

## Camión con tarimas

Para el caso práctico **camión → N tarimas → cajas → CEDI**, ver [33 — Ingreso CEDI por camión](./33-cedi-inbound-trucks.md) (`inbound_shipments` / migración `026`).

## Fuera de alcance (siguiente)

- Traslado CEDI → tienda (API atómica de transfer)
- Parseo CFDI / XML
- Ubicaciones / pasillos dentro del CEDI
- Papeletas de identificación entre tiendas → ver [23 — Papeletas](./23-shipping-slips.md)
