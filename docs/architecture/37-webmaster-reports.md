# 37 — Reportes webmaster (impresión y digital)

El rol **`webmaster`** genera reportes **impresos** (`window.print`) o **digitales** (CSV / JSON) de casi todo lo administrado en la sucursal.

## Migración

`infra/postgres/migrations/030_webmaster_reports.sql`

| Permiso | Uso |
|---|---|
| `reporting.catalog.read` | Ver el catálogo de reportes |
| `reporting.print` | Imprimir hoja de reporte |
| `reporting.export` | Descargar CSV / JSON |

También se otorgan lecturas amplias (inventario, POS, clientes, nómina, seguridad, compras, instalaciones) al rol `webmaster`. Liderazgo (`store_admin`, `regional_manager`, etc.) recibe export/print.

## SPA

- Ruta: `/reports/webmaster`
- Nav: **Reportes webmaster**
- Persona demo: `POST /auth/dev-token?persona=webmaster`

### Catálogo de reportes

Existencias, movimientos, merma/ajustes, recepciones, traslados, papeletas, transporte, paquetería, garantías, devoluciones, ventas POS, clientes, empleados, nómina, seguridad/andén, etiquetas, catálogo.

## AuthZ

OPA + fallback local: `reporting.export` / `reporting.print` / `reporting.catalog.read`.

## Notas

- Impresión: CSS `@media print` sobre `.wm-print-sheet` (mismo patrón que papeletas/transporte).
- Digital: descarga en el navegador (sin motor PDF servidor).
- Los datos salen de los endpoints de lectura existentes vía gateway.
