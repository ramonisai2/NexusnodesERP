#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DATABASE_URL="${DATABASE_URL:-postgres://nexus:nexus@127.0.0.1:5432/nexus_erp?sslmode=disable}"
MODE="${1:-up}"

if [[ "$MODE" != "up" ]]; then
  echo "usage: migrate.sh up"
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found. With docker-compose, migrations auto-apply on first postgres boot."
  exit 1
fi

for f in "$ROOT"/infra/postgres/migrations/*.sql; do
  echo "Applying $(basename "$f")..."
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f"
done
echo "Migrations complete."
