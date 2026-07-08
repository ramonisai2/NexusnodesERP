# 12 — Etiquetadora: precios y datos por tienda

## Objetivo

Cada tienda (sucursal) puede imprimir etiquetas con:

- **Descripción pública** (texto para el cliente)
- **Material** (código único del artículo)
- **Código de barras** (EAN/UPC)
- **Talla** y **color**
- **Marca** / otras descripciones
- Precios por tienda:
  - **Común** — precio regular
  - **Especial** — promoción
  - **Final** — descontinuación hasta agotar existencias

El modo activo (`price_mode`) decide qué precio imprime la etiquetadora.

## Modelo

```
product_skus
  material_code, barcode, size_code, color_code, brand, extra_attrs

store_sku_labels  (por branch + sku)
  store_display_name   → "LA MARINA"
  public_description   → "CAMISETA MANGA CORTA DE HOMBRE"
  department_label     → "ROPA DEPORTIVA"
  common_price / special_price / final_price
  price_mode           → COMMON | SPECIAL | FINAL
  effective_price      → derivado del modo
```

## API

- `GET /inventory/labels?branch_id=&sku=&department=`
- Respuesta incluye `effective_price` y todos los campos listos para la impresora.

## Demo

Camiseta Club América (`CAAL686101YL1` / barcode `7450130556398`):

| Tienda | Modo | Efectivo |
|---|---|---|
| Norte (LA MARINA) | COMMON | 899 |
| Sur (LA MARINA SUR) | SPECIAL | 699 |
| Figura coleccionable Norte | FINAL | 299 (hasta agotar) |
