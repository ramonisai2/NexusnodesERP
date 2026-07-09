# AGENTS.md

## Cursor Cloud specific instructions

NexusERP is a monorepo ERP (inventory + payroll) made of Go microservices (`go.work`)
and a React + Vite SPA (`apps/web`, pnpm workspace). Standard commands live in the
`Makefile`, root `package.json` scripts, and `README.md` / `docs/architecture/`. The
environment already has Go 1.22, Node 22, and pnpm 9.15 installed; the update script
handles dependency refresh (`pnpm install` + `go mod download`).

### Services and ports
- `web` (Vite dev) — :5173, proxies `/api/*` → gateway :8080 (see `apps/web/vite.config.ts`).
- `gateway` — :8080 (JWT + proxy). `inventory` — :8082. `payroll` — :8083.
- Optional: `reporting-bff` :8084, `reports` :8085, `search` :8086, `messaging` :8087,
  plus `outbox-relay` / `notification` (need NATS). Run each with the matching `make run-*` target.

### Non-obvious gotchas
- **Services default to Postgres + OPA and exit hard if `DATABASE_URL` is set but the DB
  is unreachable.** The `Makefile` `run-*` targets inject a default `DATABASE_URL`/`OPA_URL`,
  so a bare `make run-inventory` fails when no Postgres/OPA is up.
- **Two dev modes:**
  - *In-memory (no infra):* run with the vars emptied, e.g.
    `make run-inventory DATABASE_URL= OPA_URL=`. Empty `DATABASE_URL` → seeded in-memory
    store; empty `OPA_URL` → local authz policy fallback (`packages/go/authz`);
    `DEV_AUTH_BYPASS=true` (already in the target) auto-populates the request subject.
  - *Full stack (needed for the SPA dashboard):* run Postgres, apply migrations, then run
    services in default mode (`make run-inventory OPA_URL=` to keep local authz without OPA).
- **The SPA gates the whole authenticated app behind `/setup/status`.** Without Postgres,
  `needs_setup` is always `true`, so login redirects to the setup wizard — and the wizard's
  `POST /setup/complete` itself requires a DB. So to reach the dashboard you MUST have
  Postgres with migrations applied. Migration `013_store_setup.sql` marks the install
  complete once the seeded `DEMO` org from `004_seed.sql` exists, so after `make migrate`
  the SPA goes straight to the dashboard with demo data.
- **Postgres is not in the base image.** Bring it up the repo's way (`docker compose up -d postgres`,
  which auto-applies `infra/postgres/migrations` on first boot) if Docker is available, or
  install Postgres natively, create role/db `nexus`/`nexus_erp`, and run `make migrate`
  (uses `psql`; `DATABASE_URL=postgres://nexus:nexus@127.0.0.1:5432/nexus_erp?sslmode=disable`).
- **Login / demo data:** on the login screen use "Explore demo mode" or "Sign in as owner"
  (dev-token personas). The owner/admin persona defaults to branch `br_cedi`, which has **no**
  seeded stock — switch the top-bar **Branch** dropdown to **Norte** (`br_norte`) to see the
  seeded inventory (e.g. `BOLT-M8`).
- **`pnpm lint:web` and `pnpm build:web` currently FAIL** due to pre-existing TypeScript
  errors in a few pages (`tsc --noEmit`). This is a source-code issue, not an environment
  problem. The Vite dev server (`make run-web`) uses esbuild and runs fine regardless, so
  UI development/testing is unaffected.
- Go tests: `make test-go` (all pass, no infra needed). DB-backed store tests: `make test-go-pg`
  (needs Postgres).
