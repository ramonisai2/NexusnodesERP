# 05 — Persistencia, Concurrencia y Sincronización

## 1. Estrategia de datos

| Dato | Consistencia | Store |
|------|--------------|-------|
| Stock levels, movements | Fuerte (ACID) | PostgreSQL primary |
| Payroll runs / entries | Fuerte (ACID) | PostgreSQL primary |
| Sesiones / JWT deny | Eventual corta | Redis |
| Audit trail | Append-only fuerte | PostgreSQL particionado (+ archive S3) |
| Reportes / BI | Eventual | Réplica + warehouse |
| Documentos (recibos, PDF nómina) | Eventual | S3 + metadatos en PG |

## 2. Replicación

```
Primary (AZ-a)
   │ streaming sync
   ├──────────────▶ Hot standby (AZ-b)   ← failover automático
   │ streaming async
   └──────────────▶ DR replica (AZ-c)    ← PITR + backups base diarios
```

- Failover: Patroni / CloudNativePG / RDS Multi-AZ.
- Réplicas de solo lectura para `reporting-service` y listados pesados.
- **Nunca** escribir en réplicas.

## 3. Concurrencia

### Optimistic locking (default)
Campos `version` en `stock_level`, `stock_movement`, `payroll_run`, `user_account`.

```sql
UPDATE stock_level
SET qty_on_hand = qty_on_hand - :qty,
    version = version + 1,
    updated_at = now()
WHERE warehouse_id = :wh AND sku_id = :sku AND version = :ver
  AND qty_on_hand - qty_reserved >= :qty;
-- rowcount = 0 → 409 Conflict
```

### Pessimistic locking (casos críticos)
- Posting de transferencia que toca 2 warehouses en la misma DB: `SELECT … FOR UPDATE` ordenado por `warehouse_id` para evitar deadlocks.
- Cálculo de `payroll_run`: soft lock (`lock_owner`, `lock_expires_at`) + unique parcial `WHERE status IN ('calculated','approved')`.

### Idempotencia
Header `Idempotency-Key` persistido por tenant; reintentos seguros en redes inestables y consumers Kafka.

## 4. Sincronización asíncrona

```
[Service TX] → outbox_table → publisher → Kafka → consumers
                                      ├─ audit-service
                                      ├─ notification-service
                                      ├─ reporting projector
                                      └─ branch sync (si multi-región)
```

### Multi-sucursal / multi-región
- **Single primary region** preferida para nómina e inventario (evita conflictos de stock).
- Si se requiere activo-activo por región: CRDTs **no** aplican a stock monetario; usar **ownership por warehouse** (cada SKU-warehouse tiene región dueña) y transferencias como sagas.

### Saga ejemplo: Transferencia inter-sucursal
1. Reservar stock origen (compensable).
2. Crear movimiento tránsito.
3. Confirmar recepción destino.
4. Liberar / commit. Compensación: liberar reserva si timeout.

## 5. Integridad referencial y reglas

- FK estrictas dentro del mismo bounded context.
- Entre servicios: **sin FK cross-DB**; integridad por IDs + verificación asíncrona / anti-corruption layer.
- Triggers mínimos; lógica de negocio en aplicación.
- Check constraints para invariantes simples (`qty >= 0`, estados válidos).

## 6. Backup y recuperación

| Ítem | Política |
|------|----------|
| Full + WAL | Continuo; retención 30–90 días |
| Restore drill | Trimestral |
| Object storage | Versionado + Object Lock (compliance) |
| Secrets de backup | Segregados del prod runtime |
