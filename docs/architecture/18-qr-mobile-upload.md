# 18 — Subida móvil por QR

## Objetivo

Desde la computadora (Fotos) se genera un **QR de corta vida**. Cualquier teléfono lo escanea, abre una página pública y **elige archivos de su galería** para el reporte — sin iniciar sesión en el celular.

## Flujo

```
SPA /reports/images (JWT)
  → POST /reports/images/upload-sessions
      → token opaco (solo hash en BD), TTL ~20 min, máx. N fotos
  → QR apunta a {origin}/upload/{token}

Teléfono (sin JWT)
  → GET  /api/reports/images/upload/{token}   # estado / cupos
  → POST /api/reports/images/upload/{token}   # multipart files
      → claim cupos → downscale → image_reports
```

El gateway expone `GET/POST /reports/images/upload/{token}` **antes** del middleware JWT. Crear sesión sigue exigiendo `reporting.image.create`.

## Seguridad

| Medida | Detalle |
|---|---|
| Token | 24 bytes aleatorios; en BD solo `sha256` |
| TTL | `IMAGE_UPLOAD_SESSION_TTL_MIN` (default 20) |
| Cupo | `max_files` (default 8, tope 20); claim atómico |
| Autoría | `created_by` / org / branch de quien generó el QR |
| RLS | tabla `image_upload_sessions` con aislamiento por org |

No se pone un JWT en el QR: el teléfono solo conoce el token de sesión.

## API

| Método | Ruta | Auth |
|---|---|---|
| `POST /reports/images/upload-sessions` | JSON `branch_id`, `title_hint?`, `max_files?` | JWT + `reporting.image.create` |
| `GET /reports/images/upload/{token}` | estado | público |
| `POST /reports/images/upload/{token}` | multipart `files` / `file`, `title?`, `notes?` | público |

## Config

```bash
PUBLIC_WEB_BASE=http://localhost:5173   # URL canónica en la respuesta API (el QR de la SPA usa window.location.origin)
IMAGE_UPLOAD_SESSION_TTL_MIN=20
```

## SPA

- Escritorio: panel **Generar QR** en `/reports/images`.
- Móvil: ruta pública `/upload/:token` (fuera de `RequireAuth`).
