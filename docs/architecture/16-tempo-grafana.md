# 16 — Tempo + Grafana (trazas y observabilidad)

## Objetivo

Exportar trazas OpenTelemetry de los microservicios NexusERP a **Tempo** y explorarlas en **Grafana** con dashboards TraceQL provisionados.

## Arquitectura

```
Go services (otelx)
  OTEL_EXPORTER=otlp
  OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318
        │
        ▼  OTLP/HTTP
┌─────────────────┐     OTLP/gRPC      ┌──────────┐
│ otel-collector  │ ─────────────────► │  Tempo   │
│ (opcional)      │                    │  :3200   │
└─────────────────┘                    └────┬─────┘
                                            │
                                       ┌────▼─────┐
                                       │ Grafana  │
                                       │  :3000   │
                                       └──────────┘
```

Sin collector: los servicios pueden apuntar **directo a Tempo** en `:4318` (puerto OTLP HTTP del contenedor Tempo).

## Arranque (Docker)

```bash
docker compose --profile obs up -d tempo grafana
# Opcional: collector en host :4319 → Tempo
docker compose --profile obs up -d otel-collector

# Apps
export OTEL_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318
make run-gateway run-inventory run-payroll run-reports run-reporting run-relay run-notification
```

- Grafana: http://localhost:3000 (`admin` / `admin`, anónimo Viewer)
- Tempo API: http://localhost:3200/ready

## Arranque sin Docker (binarios)

```bash
# Tempo
mkdir -p /tmp/tempo-data
tempo -config.file=infra/observability/tempo-local.yaml

# Grafana OSS (opcional) con provisioning montado, o Explore vía Tempo API
```

Makefile:

```bash
make obs-up      # docker compose --profile obs up -d
make obs-down
make run-gateway-otel   # gateway con OTLP hacia localhost:4318
```

## Propagación W3C

| Tramo | Mecanismo |
|---|---|
| Gateway → inventory/payroll/reports/BFF | `otelx.InjectHTTP` en reverse proxy |
| reporting-bff → upstream | `otelx.HTTPClient` (`otelhttp` transport) |
| outbox-relay → NATS | headers `traceparent` en `PublishMsg` |
| NATS → notification | `otelx.ExtractMap` al crear el span |

## Dashboards provisionados

| UID | Título | Contenido |
|---|---|---|
| `nexus-service-traces` | Service Traces | Gateway, inventory, payroll, reports + errores |
| `nexus-event-pipeline` | Event Pipeline | outbox-relay, notification, BFF, lentos >500ms |

Carpeta Grafana: **NexusERP**.

## TraceQL útiles

```traceql
{ resource.service.name = "gateway" }
{ resource.service.name = "inventory" && name =~ "POST.*" }
{ resource.service.name = "notification" && name = "notification.handle" }
{ status = error }
{ duration > 500ms }
```

## Archivos

```
infra/observability/
  tempo.yaml
  tempo-local.yaml
  otel-collector-config.yaml
  grafana/provisioning/datasources/tempo.yaml
  grafana/provisioning/dashboards/dashboards.yaml
  grafana/dashboards/*.json
```

## Verificación rápida

1. `curl -sf http://localhost:3200/ready`
2. Generar tráfico: `GET /healthz`, movimiento de inventario, prepare nómina.
3. Grafana → Explore → Tempo → TraceQL `{ resource.service.name = "gateway" }`
4. Abrir dashboards en carpeta NexusERP.
