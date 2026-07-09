#!/usr/bin/env bash
# NexusERP installer — bundles runtime dependencies via Docker.
# Requirement: Docker Desktop / Docker Engine + Compose v2.
# You do NOT need local Go, Node, pnpm, Postgres, or OPA.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

WEB_PORT="${WEB_PORT:-8088}"
COMPOSE=(docker compose --profile app)

red() { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
bold() { printf '\033[1m%s\033[0m\n' "$*"; }

need_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    red "Docker no está instalado."
    echo "Instálalo desde https://docs.docker.com/get-docker/ y vuelve a ejecutar:"
    echo "  ./infra/scripts/install.sh"
    exit 1
  fi
  if ! docker compose version >/dev/null 2>&1; then
    red "Docker Compose v2 no está disponible (comando: docker compose)."
    exit 1
  fi
  if ! docker info >/dev/null 2>&1; then
    red "Docker no está corriendo. Abre Docker Desktop (o el daemon) e inténtalo de nuevo."
    exit 1
  fi
}

ensure_env() {
  if [[ ! -f .env ]]; then
    cp .env.example .env
    # Point browser-facing URLs at the bundled web port.
    if grep -q '^PUBLIC_WEB_BASE=' .env; then
      sed -i.bak "s|^PUBLIC_WEB_BASE=.*|PUBLIC_WEB_BASE=http://localhost:${WEB_PORT}|" .env && rm -f .env.bak
    else
      echo "PUBLIC_WEB_BASE=http://localhost:${WEB_PORT}" >> .env
    fi
    if grep -q '^CORS_ORIGIN=' .env 2>/dev/null; then
      sed -i.bak "s|^CORS_ORIGIN=.*|CORS_ORIGIN=http://localhost:${WEB_PORT}|" .env && rm -f .env.bak
    else
      echo "CORS_ORIGIN=http://localhost:${WEB_PORT}" >> .env
    fi
    echo "WEB_PORT=${WEB_PORT}" >> .env
    green "Creado .env desde .env.example (PUBLIC_WEB_BASE → :${WEB_PORT})"
  fi
}

wait_http() {
  local url="$1" name="$2" tries="${3:-60}"
  local i=0
  until curl -fsS "$url" >/dev/null 2>&1; do
    i=$((i + 1))
    if (( i >= tries )); then
      red "Timeout esperando $name ($url)"
      "${COMPOSE[@]}" ps
      exit 1
    fi
    sleep 2
  done
  green "OK $name"
}

cmd="${1:-up}"

case "$cmd" in
  up|install|start)
    bold "NexusERP — instalador con dependencias (Docker)"
    need_docker
    ensure_env
    export WEB_PORT
    export PUBLIC_WEB_BASE="${PUBLIC_WEB_BASE:-http://localhost:${WEB_PORT}}"
    export CORS_ORIGIN="${CORS_ORIGIN:-http://localhost:${WEB_PORT}}"
    bold "1/4 Bajando imágenes base (Postgres, OPA)…"
    docker compose up -d postgres opa
    bold "2/4 Esperando Postgres…"
    for i in $(seq 1 60); do
      if docker compose exec -T postgres pg_isready -U nexus -d nexus_erp >/dev/null 2>&1; then
        break
      fi
      if (( i == 60 )); then
        red "Postgres no respondió a tiempo"
        exit 1
      fi
      sleep 2
    done
    green "OK postgres"
    bold "3/4 Aplicando migraciones SQL (idempotente)…"
    for f in infra/postgres/migrations/*.sql; do
      echo "  → $(basename "$f")"
      docker compose exec -T postgres \
        psql -U nexus -d nexus_erp -v ON_ERROR_STOP=1 -f - <"$f" >/dev/null
    done
    green "OK migraciones"
    bold "4/4 Construyendo e iniciando apps (inventory, payroll, gateway, web)…"
    echo "   (la primera vez descarga Go/Node y puede tardar varios minutos)"
    WEB_PORT="$WEB_PORT" PUBLIC_WEB_BASE="$PUBLIC_WEB_BASE" CORS_ORIGIN="$CORS_ORIGIN" \
      "${COMPOSE[@]}" up -d --build
    bold "Esperando salud…"
    wait_http "http://127.0.0.1:8080/healthz" "gateway"
    wait_http "http://127.0.0.1:${WEB_PORT}/" "web"
    echo
    green "Listo."
    echo
    echo "  Abrir:     http://localhost:${WEB_PORT}"
    echo "  Demo:      botón «Explorar modo demo»"
    echo "  Setup:     http://localhost:${WEB_PORT}/setup"
    echo "  Vitrina:   http://localhost:${WEB_PORT}/tienda/demo-norte"
    echo "  Gateway:   http://localhost:8080/healthz"
    echo
    echo "  Parar:     ./infra/scripts/install.sh down"
    echo "  Logs:      ./infra/scripts/install.sh logs"
    echo "  Estado:    ./infra/scripts/install.sh status"
    echo
    echo "Dependencias incluidas en Docker: Postgres, OPA, Inventory, Payroll, Gateway, SPA."
    echo "No necesitas instalar Go ni Node en tu máquina."
    ;;
  down|stop)
    need_docker
    "${COMPOSE[@]}" down
    docker compose stop postgres opa 2>/dev/null || true
    green "Servicios detenidos."
    ;;
  destroy)
    need_docker
    "${COMPOSE[@]}" down -v
    docker compose down -v
    green "Servicios y volúmenes eliminados."
    ;;
  logs)
    need_docker
    "${COMPOSE[@]}" logs -f --tail=100
    ;;
  status|ps)
    need_docker
    docker compose ps
    "${COMPOSE[@]}" ps
    ;;
  rebuild)
    need_docker
    ensure_env
    WEB_PORT="$WEB_PORT" "${COMPOSE[@]}" up -d --build --force-recreate
    green "Rebuild completo."
    ;;
  *)
    echo "Uso: $0 {up|down|destroy|logs|status|rebuild}"
    echo
    echo "  up       Instala dependencias (imágenes Docker), construye y arranca todo"
    echo "  down     Detiene contenedores (conserva datos de Postgres)"
    echo "  destroy  Detiene y borra volúmenes"
    echo "  logs     Sigue logs del stack app"
    echo "  status   Lista contenedores"
    echo "  rebuild  Reconstruye imágenes de apps"
    exit 1
    ;;
esac
