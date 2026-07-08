# 15 — Reportes con imágenes (web-optimized)

## Objetivo

Los usuarios suben evidencias fotográficas (daños, anaqueles, auditorías). Al guardar, el servicio **reduce la imagen a una resolución ligera para internet** (lado máximo 1280px, JPEG ~82%) y solo persiste esa variante.

## Flujo

```
SPA /reports/images
  → POST multipart /reports/images (gateway)
    → apps/reports :8085
        1. authz reporting.image.create
        2. decode JPEG/PNG/GIF/WebP
        3. downscale (max edge 1280) + JPEG encode
        4. write blob a IMAGE_STORAGE_PATH
        5. insert metadata en image_reports
```

## API

| Método | Ruta gateway | Acción OPA |
|---|---|---|
| `POST /reports/images` | multipart: `file`, `title`, `notes?`, `branch_id` | `reporting.image.create` |
| `GET /reports/images?branch_id=` | lista metadatos | `reporting.image.read` |
| `GET /reports/images/{id}/content` | bytes JPEG optimizados | `reporting.image.read` |

## Config

```bash
REPORTS_ADDR=:8085
REPORTS_URL=http://127.0.0.1:8085
IMAGE_STORAGE_PATH=./data/image-reports
IMAGE_MAX_EDGE=1280
IMAGE_JPEG_QUALITY=82
IMAGE_MAX_UPLOAD_BYTES=12582912
```

## Arranque

```bash
make migrate
make run-reports
make run-gateway   # proxy /reports/*
```

SPA: menú **Fotos** → `/reports/images`.

Para subir desde el teléfono con QR (sin login en el celular), ver [18 — Subida móvil por QR](./18-qr-mobile-upload.md).
