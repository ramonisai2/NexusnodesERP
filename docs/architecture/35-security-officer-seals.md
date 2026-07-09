# 35 — Seguridad física: sellos y reportes de vigilancia

## Objetivo

Dar a **empleados de seguridad / vigilancia** acceso de solo lectura a movimientos logísticos (salidas, llegadas, contenedores) y la capacidad de **verificar sellos/precintos** en andén, sin poder mutar existencias.

## Rol

`security_officer` — permisos:

| Permiso | Uso |
|---|---|
| `reporting.security.read` | Reporte unificado de vigilancia |
| `inventory.seal.verify` | Verificar sello en hoja de transporte o camión CEDI |
| `inventory.slip.read` / `transport.read` / `parcel.read` / `transfer.read` / `shipment.read` | Lectura logística |
| `reporting.image.read` / `create` | Evidencia fotográfica |

Persona demo: `POST /auth/dev-token?persona=security`

## Sellos

Columnas en `transport_sheets` y `inbound_shipments`:

- `seal_number`, `seal_status` (`APPLIED` \| `VERIFIED` \| `BROKEN` \| `MISSING`)
- `seal_verified_at`, `seal_verified_by`, `seal_notes`

Al crear hoja/camión se puede capturar el sello (`APPLIED`). Seguridad confirma en andén.

## API

| Método | Ruta | Permiso |
|---|---|---|
| `GET /inventory/security-logistics` | timeline TRANSPORT + INBOUND + SLIP | `reporting.security.read` |
| `POST /inventory/transport-sheets/{id}/verify-seal` | verificar sello salida | `inventory.seal.verify` |
| `POST /inventory/inbound-shipments/{id}/verify-seal` | verificar sello llegada CEDI | idem |

Query del reporte: `branch_id`, `kind`, `seal_only=1`, `limit`.

## SPA

`/reports/seguridad` — menú **Seguridad**.

## Migración

`028_security_officer_seals.sql`

## Fuera de alcance

- Sellos por tarima/caja individual
- Lectores RFID / escáner de precinto
- Turnos de vigilancia / bitácora de rondines
