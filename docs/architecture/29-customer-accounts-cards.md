# 29 — Cuentas de cliente y tarjetas (chip / código de barras)

## Objetivo

Que los **clientes finales** puedan:

1. **Registrarse** con correo y contraseña desde la tienda en línea (`/tienda/{slug}/cuenta`).
2. **Gestionar tarjetas** propias: código de barras (`BARCODE`) o chip/NFC (`CHIP`).
3. Que el **personal de tienda** busque clientes, emita tarjetas y las bloquee.

## Modelo

| Tabla | Rol |
|---|---|
| `customers` | Cuenta por org (`email` único), hash bcrypt, sucursal preferida |
| `customer_cards` | Tarjetas `BARCODE` \| `CHIP`, código único por org, estados ACTIVE/BLOCKED/LOST/EXPIRED |

## Flujos

```
Público:  POST /customers/register|login  → gateway valida en inventory y emite JWT de cliente
Cliente:  GET/POST /customers/me(/cards)  · POST …/cards/{id}/block
Staff:    GET /customers · GET /customers/{id}
          POST /customers/{id}/cards · GET /customers/cards/lookup?code=
          POST /customers/cards/{id}/block
```

JWT de cliente: rol `customer`, permisos `customer.self.read` + `customer.card.self`, attr `customer_id`.

## SPA

| Ruta | Quién |
|---|---|
| `/tienda/{slug}/cuenta` | Cliente (público + sesión propia) |
| `/customers` | Staff (PolicyGuard) |

Enlace **Mi cuenta** en la vitrina pública.

## Migración

`infra/postgres/migrations/023_customer_accounts.sql`

## Relación

- [27 — Tienda en línea](./27-configurable-storefront.md): el slug publicado define la org del registro
- [02 — Seguridad](./02-security-rbac-abac.md): JWT + OPA; clientes no usan el IdP de staff
