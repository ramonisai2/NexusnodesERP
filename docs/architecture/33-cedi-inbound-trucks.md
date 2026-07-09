# 33 — Ingreso CEDI por camión (tarimas y cajas)

## Caso práctico

Un camión de proveedor llega al CEDI con **8 tarimas**. Cada tarima trae **cajas** de uno o varios SKUs. El operador captura el camión, cuenta tarima por tarima y confirma el ingreso; el stock queda en el almacén CEDI.

```
Camión proveedor
  → 8 tarimas (TAR-{sucursal}-01 … 08)
      → cajas (SKU × núm. cajas × unidades/caja)
  → POST inbound shipment
  → resumen por SKU → receipt + movimiento RECEIPT
  → stock en cedi_centro
```

## Modelo

| Tabla | Rol |
|---|---|
| `inbound_shipments` | Cabecera del camión (proveedor, factura, placas, andén, N tarimas) |
| `inbound_pallets` | Tarimas numeradas con código `TAR-…` |
| `inbound_boxes` | Líneas de caja/SKU por tarima (`boxes_count × units_per_box = quantity`) |
| `inbound_receipts.shipment_id` | Vínculo a la recepción plana que mueve stock |

Estados del envío: `SCHEDULED` → `ARRIVED` → `RECEIVING` → `POSTED` | `CANCELLED`.

## API

Prefijo vía gateway: `/inventory/…`

| Método | Ruta | Permiso |
|---|---|---|
| `GET /inbound-shipments` | listar camiones | `inventory.shipment.read` (o receipt/balance) |
| `GET /inbound-shipments/{id}` | detalle + tarimas + cajas + resumen SKU | idem |
| `POST /inbound-shipments` | alta con pallets/boxes | `inventory.shipment.create` |
| `POST /inbound-shipments/{id}/post` | confirma → crea/posta receipt | `inventory.shipment.post` |

Al postear se reutiliza el flujo de [20 — recepción](./20-cedi-inbound-receiving.md): `CreateReceipt` + `PostReceipt` (balances, movimientos, etiquetas, outbox `InboundShipmentPosted`).

## SPA

`/inventory/receiving` — modo **Camión / tarimas** (por defecto) o **Factura simple**.

Caso demo: 8 tarimas, cajas por SKU, destino `cedi_centro`.

## Migración

`026_cedi_inbound_trucks.sql` — tablas, RLS, permisos `inventory.shipment.*` en roles de almacén.

## Fuera de alcance

- Escaneo de códigos de tarima/caja en piso
- Cross-dock / putaway a pasillo
- Devolución parcial por tarima
