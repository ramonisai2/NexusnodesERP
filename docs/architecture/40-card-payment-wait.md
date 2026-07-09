# 40 — Espera de pagos con tarjeta (terminal bancario)

## Objetivo

Cuando el cliente paga con **tarjeta de banco**, la caja no debe descontar inventario ni cerrar el ticket hasta que el **terminal** autorice (o rechace) el cobro. Este módulo agrega una **cola de espera** entre el carrito POS y `CompleteSale`.

## Módulo de instalación

| Código | Default | Nav |
|---|---|---|
| `card_payments` | **OFF** | `nav.cardWaits` → `/caja/espera-tarjeta` |

Se desbloquea en el wizard de primer uso (paso Funciones) o queda libre en org DEMO (`enabled_modules = NULL`).

Permisos del módulo:

- `pos.card.wait.read`
- `pos.card.wait.create`
- `pos.card.wait.confirm`
- `pos.card.wait.cancel`

## Flujo

```text
Caja (método CARD + módulo ON)
  → POST /pos/card-waits          status=WAITING  (sin ISSUE de stock)
  → Cola /caja/espera-tarjeta
      → confirm approved=true     → CompleteSale (CARD) + status=APPROVED
      → confirm approved=false    → status=DECLINED (sin venta)
      → cancel                    → status=CANCELLED (sin venta)
```

Idempotencia de venta al aprobar: `cardwait-{wait_id}`.

## Tabla (`033_card_payment_wait.sql`)

`pos_card_payment_waits` — carrito JSON, monto, datos de factura opcionales, `terminal_ref`, `auth_code`, `sale_id`, estados `WAITING|APPROVED|DECLINED|CANCELLED|EXPIRED`.

## API (gateway `/pos/*` → inventory)

| Método | Ruta | Permiso |
|---|---|---|
| GET | `/pos/card-waits?branch_id=&status=` | `pos.card.wait.read` |
| GET | `/pos/card-waits/{id}` | `pos.card.wait.read` |
| POST | `/pos/card-waits` | `pos.card.wait.create` |
| POST | `/pos/card-waits/{id}/confirm` | `pos.card.wait.confirm` |
| POST | `/pos/card-waits/{id}/cancel` | `pos.card.wait.cancel` |

## SPA

- **Caja** (`/caja`): con módulo + permiso `create`, pagar con tarjeta encola en vez de completar al instante.
- **Cola** (`/caja/espera-tarjeta`): aprueba (código de autorización), rechaza o cancela; auto-refresh cada 4 s en `WAITING`.

## Roles

Cajero / admin / coordinador / webmaster: create + confirm + cancel.  
Asociado de ventas: create + read (sin confirm).

## Relación con POS

Ver [32 — Caja POS + IVA](./32-pos-caja-iva.md). Sin el módulo `card_payments`, CARD sigue completando la venta de inmediato (comportamiento anterior).
