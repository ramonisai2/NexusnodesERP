# Bootstrap local (pasos 1–2)

## Qué hay en este slice

| Pieza | Ubicación | Estado |
|---|---|---|
| Monorepo pnpm + Go work | raíz | listo |
| API Gateway (JWT, CORS, proxy) | `apps/gateway` | listo (dev HS256) |
| Inventory service | `apps/inventory` | vertical slice in-memory |
| Payroll service | `apps/payroll` | vertical slice + SoD |
| SPA React + PolicyGuard | `apps/web` | listo |
| Migraciones PostgreSQL | `infra/postgres/migrations` | listo |
| Políticas OPA | `infra/opa/policies` | listo |
| Realm Keycloak | `infra/keycloak` | listo |
| docker-compose deps | `docker-compose.yml` | listo |

## Arranque rápido (sin Docker para APIs)

```bash
cp .env.example .env
make deps
make test-go

# terminales separadas
make run-inventory
make run-payroll
make run-gateway
make run-web
```

Abre http://localhost:5173 → **Entrar como Analista** o **Aprobador**.

### Probar SoD de nómina

1. Analista: Inventario / Nómina → **Preparar corrida**
2. Salir → entrar como **Aprobador** → **Aprobar**
3. Si el mismo `sub` prepara y aprueba → `403 sod_violation`

### Dependencias opcionales (Docker)

```bash
docker compose up -d postgres redis opa keycloak
```

## Auth en este bootstrap

- **Dev:** `POST /auth/dev-token?persona=analyst|approver` (solo con `DEV_AUTH_BYPASS=true`)
- **Prod path:** Keycloak realm `nexus` + PKCE; gateway validará JWKS (pendiente de cablear RS256)
- **AuthZ:** permisos en JWT + chequeo de sucursal en servicios; OPA listo en `:8181`

## Siguiente (paso 3+)

1. Sustituir stores in-memory por PostgreSQL (`pgx`)
2. Validación JWT via JWKS de Keycloak
3. PEP → OPA en cada comando de escritura
4. Outbox + Kafka para proyecciones
