# 32 — Caja de cobro (POS) con IVA MX y opción de facturar

## Objetivo

Una **caja amigable** para tienda: escanear productos, cobrar, imprimir recibo con desglose de **IVA 16%** (leyes mexicanas) y, si el cliente lo pide, **solicitar factura (CFDI)** con RFC / uso CFDI.

## Qué incluye (MVP)

| Capacidad | Detalle |
|---|---|
| Ticket | Folio por sucursal (`T-BR_NORTE-000001`) |
| Precios | Reutiliza `store_sku_labels` (MXN) |
| IVA | Default **16%**; precios **con IVA incluido** (típico abarrotes) |
| Pagos | Efectivo (con cambio), tarjeta, transferencia |
| Stock | Al completar: movimientos `ISSUE` por línea |
| Factura | Solicitud CFDI guardada (`REQUESTED`); timbrado PAC = fase 2 |
| Recibo | Vista imprimible en SPA (`/caja`) |

## Tablas (`025_pos_sales.sql`)

- `branch_fiscal_settings` — RFC emisor, régimen, CP, `prices_include_tax`, series/folios
- `pos_sales` / `pos_sale_lines` / `pos_payments`
- `fiscal_invoices` — solicitud de CFDI ligada al ticket
- `customer_fiscal_profiles` — reservado para perfiles fiscales de cliente
- `inventory_movements.source_sale_id`

## API (vía gateway `/pos/*` → inventory)

| Método | Ruta | Permiso |
|---|---|---|
| `GET` | `/pos/fiscal-settings?branch_id=` | `pos.sale.read` |
| `PUT` | `/pos/fiscal-settings` | `pos.settings.manage` |
| `POST` | `/pos/sales/complete` | `pos.sale.create` |
| `GET` | `/pos/sales` | `pos.sale.read` |
| `GET` | `/pos/sales/{id}` | `pos.sale.read` |
| `POST` | `/pos/sales/{id}/invoice` | `pos.invoice.request` |

### Cobro (ejemplo)

```json
{
  "branch_id": "br_norte",
  "lines": [{ "sku": "BOLT-M8", "quantity": 2 }],
  "payments": [{ "method": "CASH", "amount": 46.4, "received_amount": 50 }],
  "request_invoice": true,
  "invoice_rfc": "XAXX010101000",
  "invoice_name": "Público en general",
  "invoice_uso_cfdi": "G03",
  "idempotency_key": "caja-demo-1"
}
```

Desglose en ticket: **subtotal (base)** + **IVA** + **total**. Si `prices_include_tax=true`, el precio de anaquel ya trae IVA y se desglosa hacia atrás: `base = total / 1.16`.

## UI

Ruta **`/caja`** (nav “Caja”): carrito, cobro, checkbox “quiere factura”, historial e impresión.

Con el módulo de instalación **`card_payments`** activo, el método **Tarjeta** encola en `/caja/espera-tarjeta` hasta autorización del terminal (ver [40 — Espera pagos tarjeta](./40-card-payment-wait.md)).

## Fuera de alcance (siguiente)

- Timbrado real con PAC (XML/PDF UUID)
- Cancelación CFDI / notas de crédito
- Arqueo de caja / corte Z
- Propina y pagos mixtos avanzados
- Integración directa con SDK de terminal (hoy la autorización se confirma en la cola)
