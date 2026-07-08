# 24 — Hojas de transporte (notas por departamento)

## Objetivo

Una **hoja de transporte** acompaña el envío entre tiendas y concentra **notas de diversos departamentos** (Electrónica, Juguetería, …). Opcionalmente enlaza **papeletas** (`shipping_slips`) ya pegadas en contenedores.

```
Hoja HT-XXXXXXXX
  Origen → Destino · transportista / vehículo / chofer
  ├── Electrónica — notas + papeletas PAP-…
  ├── Juguetería — notas + papeletas PAP-…
  └── …
```

## Modelo

| Tabla | Rol |
|---|---|
| `transport_sheets` | Cabecera del documento (`HT-…`) |
| `transport_sheet_sections` | Bloque por `department_code` + notas |
| `transport_sheet_slip_links` | Papeletas adjuntas a un bloque |

Estados: `DRAFT` → `PRINTED` → `IN_TRANSIT` → `DELIVERED` / `CANCELLED`.

## API

| Método | Ruta | Permiso |
|---|---|---|
| `GET /inventory/transport-sheets` | listar | `inventory.transport.read` |
| `GET /inventory/transport-sheets/{id}` | detalle + secciones | `inventory.transport.read` |
| `POST /inventory/transport-sheets` | crear | `inventory.transport.create` |
| `POST /inventory/transport-sheets/{id}/print` | marcar impresa | `inventory.transport.print` |

Body de creación (ejemplo):

```json
{
  "from_branch_id": "br_norte",
  "to_branch_id": "br_sur",
  "carrier_name": "Paquetería interna",
  "sections": [
    {
      "department_code": "electronica",
      "notes": "2 cajas frágiles — no apilar",
      "slip_ids": ["…uuid papeleta…"]
    },
    {
      "department_code": "jugueteria",
      "notes": "1 bulto coleccionables"
    }
  ]
}
```

## SPA

Menú **Transporte** → `/inventory/transport`

1. Elige destino y datos del transporte.
2. Agrega bloques por departamento (notas + papeletas).
3. **Imprimir** genera la hoja para llevar con el envío.

## Relación con papeletas

- [23 — Papeletas](./23-shipping-slips.md): identificación pegada en cada contenedor.
- Esta hoja: **manifiesto** del viaje con notas por depto.

## Migración

`infra/postgres/migrations/018_transport_sheets.sql`
