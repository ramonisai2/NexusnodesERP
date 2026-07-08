#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
echo "Migrations are auto-applied via docker-compose postgres init (infra/postgres/migrations)."
echo "For an existing volume, run:"
echo "  psql \"\$DATABASE_URL\" -f $ROOT/infra/postgres/migrations/001_security.sql"
echo "  psql \"\$DATABASE_URL\" -f $ROOT/infra/postgres/migrations/002_inventory.sql"
echo "  psql \"\$DATABASE_URL\" -f $ROOT/infra/postgres/migrations/003_payroll.sql"
echo "  psql \"\$DATABASE_URL\" -f $ROOT/infra/postgres/migrations/004_seed.sql"
