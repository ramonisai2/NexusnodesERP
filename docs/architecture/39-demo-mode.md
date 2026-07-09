# 39 — Modo demo (recorrido guiado)

Permite a un visitante **ver el sistema completo** con datos de ejemplo, sin instalar su tienda.

## Entrada

Login → botón principal **Explorar modo demo** → `POST /auth/dev-token?persona=demo`

## Qué recibe

| Pieza | Valor |
|---|---|
| Usuario | `usr_dev_demo` / `demo@demo.nexus` |
| Org | `DEMO` (`org_demo`) — `profile=demo`, `enabled_modules=NULL` (todo desbloqueado) |
| Roles | `store_admin` + `hr_officer` + `security_officer` + `webmaster` |
| Sucursales | Norte, Sur, CEDI |
| UI | Banner “Modo demo” + home con atajos |

## Migración

`032_demo_mode.sql`

## Notas

- Requiere `DEV_AUTH_BYPASS=true` (igual que el resto de personas demo).
- No usa el dueño de instalación (`owner`) para no mezclar con tiendas creadas en `/setup`.
- Otros perfiles avanzados siguen disponibles para recorrer roles específicos.
