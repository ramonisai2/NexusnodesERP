# 19 — Motor de búsqueda (índice BM25)

## Objetivo

Dejar de “recorrer un almacén” (listas SQL / arreglos) para encontrar productos, etiquetas y fotos. Las consultas van a un **índice invertido con ranking BM25**, más parecido a un buscador (Google) que a un `SELECT … LIKE`.

## Arquitectura

```
OLTP (PostgreSQL)
  productos · etiquetas · image_reports
        │ reindex periódico
        ▼
apps/search :8086
  inverted index + BM25 + boosts (SKU/barcode)
        ▲
gateway /search  (JWT)
        ▲
SPA /search
```

Postgres sigue siendo la fuente de verdad transaccional. El servicio de búsqueda mantiene una **proyección de lectura** en memoria (índice), no sustituye el OLTP.

## API

| Método | Ruta gateway | Authz |
|---|---|---|
| `GET /search?q=&branch_id=&kind=&limit=` | resultados rankeados | `search.query` (o balance/catalog/image/reporting read) |
| `GET /search/stats` | tamaño del índice | `search.query` |
| `POST /search/reindex` | reconstrucción | `search.reindex` / admin |

`kind`: `product`, `label`, `photo` (coma-separados) o vacío = todos.

## Ranking

1. Tokenización con pliegue de acentos (`piña` → `pina`)
2. Prefijos cortos para typeahead / SKU parcial
3. **BM25** sobre título, subtítulo y cuerpo (título con más peso)
4. Boost exacto de **barcode** y **SKU**

## Config

```bash
SEARCH_ADDR=:8086
SEARCH_URL=http://127.0.0.1:8086
SEARCH_REINDEX_SECONDS=30
```

## Arranque

```bash
make run-search
make run-gateway   # proxy /search/*
```

SPA: menú **Buscar** → `/search`.

## Evolución

Este motor embebido evita depender de Docker/OpenSearch en local. En producción se puede sustituir el backend del índice por OpenSearch/Meilisearch manteniendo el mismo contrato HTTP `/search`.
