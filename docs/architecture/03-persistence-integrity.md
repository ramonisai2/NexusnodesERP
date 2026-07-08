# 03 — Persistencia, Integridad y Sincronización

## 1. Estrategia de datos

| Dominio | Store | Consistencia | Notas |
|---|---|---|---|
| Inventario (stock, movimientos) | PostgreSQL OLTP | Fuerte (ACID) | Particionado por `org_id` / tiempo |
| Nómina (periodos, líneas, pagos) | PostgreSQL OLTP | Fuerte (ACID) | Inmutable post-cierre |
| Catálogo / HR (lectura) | PostgreSQL + Redis | Eventual en cache | Invalidación por eventos |
| Sesiones / locks cortos | Redis Cluster | Eventual | TTL estricto |
| Eventos de dominio | Kafka / JetStream | At-least-once + idempotencia | Outbox |
| Auditoría | PostgreSQL append-only / WORM | Fuerte append | Sin UPDATE/DELETE de app |
| Reportes | Réplicas + materializadas | Eventual | GraphQL BFF |

## 2. Topología PostgreSQL

```
          ┌──────────────────┐
 Write ──►│  Primary (RW)    │── logical/physical replication ──► Replica(s) RO
          │  Patroni / Cloud │                                    │
          └────────┬─────────┘                                    ▼
                   │                                         Reporting BFF
                   ▼
            WAL / Backups (PITR)
```

- **Failover automático** (Patroni, Cloud SQL HA, RDS Multi-AZ).
- **Connection pooling** (PgBouncer / Odyssey) por servicio.
- **RLS** por `org_id` (y opcionalmente `branch_id`).

## 3. Concurrencia

### 3.1 Optimistic locking (default)

Usado en ediciones de catálogo, borradores de nómina, ajustes concurrentes de bajo conflicto:

```sql
UPDATE stock_balance
SET quantity = quantity - :qty,
    version = version + 1,
    updated_at = now()
WHERE id = :id AND version = :expected_version;
-- Si rowcount = 0 → 409 Conflict → cliente reintenta
```

### 3.2 Pessimistic locking (casos críticos)

Usado en **cierre de periodo de nómina**, **conteo físico de inventario**, **reserva de stock en checkout masivo**:

```sql
SELECT * FROM payroll_run
WHERE id = :id
FOR UPDATE NOWAIT;  -- o SKIP LOCKED en workers
```

### 3.3 Locks distribuidos (Redis)

Solo para coordinación de jobs (scheduler de nómina, sync batch), nunca como sustituto de integridad OLTP.

## 4. Integridad de inventario

**Reglas:**

1. Todo cambio de stock pasa por `inventory_movement` (append-only de hechos).
2. `stock_balance` es proyección mantenida en la misma transacción (o vía proyección confiable).
3. No se permiten balances negativos salvo política explícita (`allow_negative` por almacén).
4. Traslados = dos movimientos enlazados (`TRANSFER_OUT` / `TRANSFER_IN`) en saga local o transacción si mismo DB.
5. Reservas (`stock_reservation`) restan de `available = on_hand - reserved`.

**Estados de movimiento:** `DRAFT → POSTED → VOID` (void genera movimiento compensatorio).

## 5. Integridad de nómina

**Ciclo:**

```
Open Period → Import Attendance/Concepts → Calculate → Review
    → Approve (SoD) → Pay → Close (immutable)
```

**Reglas:**

1. Cálculo idempotente por `(run_id, employee_id, concept_code)`.
2. Tras `APPROVED`, solo `payroll_approver` distinto del preparador puede pagar.
3. Tras `CLOSED`, filas son inmutables; correcciones vía `payroll_adjustment` en periodo siguiente.
4. Montos sensibles cifrados; acceso auditado.

## 6. Sincronización asíncrona

### Outbox pattern

```
BEGIN;
  -- mutación de negocio
  INSERT INTO outbox(event_type, payload, ...) VALUES (...);
COMMIT;
-- Relay publica a Kafka y marca published_at
```

### Consumidores

| Evento | Consumidores |
|---|---|
| `InventoryMoved` | Reporting, Cache invalidation, Notification |
| `PayrollRunApproved` | Finance export, Notification, Audit mirror |
| `EmployeeUpdated` | ABAC attribute cache, Payroll eligibility |

**Garantías:** at-least-once + **idempotency keys** en consumidores (`processed_events`).

### Sync multi-sucursal

- Cada sucursal opera contra el mismo cluster lógico (cloud) con filtrado ABAC.
- Si se requiere edge offline: **CQRS local** con cola de comandos firmados y reconciliación; conflictos de stock se resuelven con políticas (last-write-wins prohibido en stock → merge manual / compensación).

## 7. Backup y DR

| Control | Objetivo |
|---|---|
| RPO | ≤ 5 min (WAL shipping / continuous backup) |
| RTO | ≤ 30 min (failover automatizado) |
| Pruebas | Restore drills trimestrales |
| Nómina | Snapshots pre-cierre + retención legal configurable |
