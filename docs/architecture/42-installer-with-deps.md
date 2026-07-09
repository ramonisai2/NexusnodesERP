# 42 — Instalador con dependencias (Docker)

## Objetivo

Que el usuario **no instale Go, Node, Postgres ni OPA a mano**.  
Un solo comando baja/construye las dependencias y deja el ERP listo.

## Requisito único

[Docker Desktop](https://docs.docker.com/get-docker/) (o Docker Engine + Compose v2).

## Uso

```bash
git clone -b cursor/erp-full-stack-bc8f https://github.com/ramonisai2/NexusnodesERP.git
cd NexusnodesERP
./infra/scripts/install.sh
# equivalente: make install
```

Abre **http://localhost:8088** → **Explorar modo demo**.

| Comando | Efecto |
|---|---|
| `./infra/scripts/install.sh` / `up` | Crea `.env`, levanta Postgres+OPA, construye e inicia apps |
| `down` | Detiene (conserva datos) |
| `destroy` | Detiene y borra volúmenes |
| `logs` / `status` / `rebuild` | Operación |

Puerto de la SPA: `WEB_PORT` (default **8088**). Gateway sigue en **8080**.

## Qué incluye el instalador

| Pieza | Cómo |
|---|---|
| PostgreSQL 16 | Imagen Docker + migraciones en primer boot |
| OPA | Imagen Docker + políticas montadas |
| Inventory / Payroll / Gateway | Build desde `apps/*/Dockerfile` |
| SPA | Build Vite → nginx, proxy `/api` → gateway |

## Archivos

- `infra/scripts/install.sh` — orquestador
- `docker-compose.yml` — perfil `app`
- `apps/gateway|inventory|payroll|web/Dockerfile`
- `infra/docker/nginx-web.conf`

## Desarrollo sin Docker de apps

Sigue válido el flujo clásico (`make run-inventory`, `make run-web`, etc.) contra `docker compose up -d postgres opa`.
