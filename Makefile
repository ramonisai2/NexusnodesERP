.PHONY: deps test-go test-go-pg run-gateway run-inventory run-payroll run-reporting run-reports run-search run-messaging run-relay run-notification run-web tidy migrate obs-up obs-down run-otel

DATABASE_URL ?= postgres://nexus:nexus@127.0.0.1:5432/nexus_erp?sslmode=disable
OPA_URL ?= http://127.0.0.1:8181
NATS_URL ?= nats://127.0.0.1:4222
OTEL_EXPORTER ?= none
OTEL_EXPORTER_OTLP_ENDPOINT ?= localhost:4318

deps:
	cd packages/go/db && go mod tidy
	cd packages/go/authz && go mod tidy
	cd packages/go/events && go mod tidy
	cd packages/go/otelx && go mod tidy
	cd apps/gateway && go mod tidy
	cd apps/inventory && go mod tidy
	cd apps/payroll && go mod tidy
	cd apps/reporting-bff && go mod tidy
	cd apps/reports && go mod tidy
	cd apps/search && go mod tidy
	cd apps/messaging && go mod tidy
	cd apps/outbox-relay && go mod tidy
	cd apps/notification && go mod tidy
	pnpm install

tidy: deps

migrate:
	bash infra/scripts/migrate.sh up

test-go:
	cd packages/go/authz && go test ./...
	cd packages/go/events && go test ./...
	cd packages/go/otelx && go test ./...
	cd packages/go/db && go test ./...
	cd apps/gateway && go test ./...
	cd apps/inventory && go test ./...
	cd apps/payroll && go test ./...
	cd apps/reporting-bff && go test ./...
	cd apps/reports && go test ./...
	cd apps/search && go test ./...
	cd apps/messaging && go test ./...
	cd apps/outbox-relay && go test ./...
	cd apps/notification && go test ./...

test-go-pg:
	DATABASE_URL='$(DATABASE_URL)' go test ./apps/inventory/internal/store ./apps/payroll/internal/store ./apps/gateway/internal/enrich ./packages/go/db -count=1

obs-up:
	docker compose --profile obs up -d tempo grafana

obs-down:
	docker compose --profile obs down

# Run any service with OTLP export (requires Tempo on :4318).
run-otel:
	@echo "Use: OTEL_EXPORTER=otlp OTEL_EXPORTER_OTLP_ENDPOINT=$(OTEL_EXPORTER_OTLP_ENDPOINT) make run-gateway"
	@echo "Grafana http://localhost:3000  Tempo http://localhost:3200/ready"

run-gateway:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' \
	OTEL_EXPORTER='$(OTEL_EXPORTER)' OTEL_EXPORTER_OTLP_ENDPOINT='$(OTEL_EXPORTER_OTLP_ENDPOINT)' \
	REPORTING_URL=http://127.0.0.1:8084 REPORTS_URL=http://127.0.0.1:8085 \
	SEARCH_URL=http://127.0.0.1:8086 MESSAGING_URL=http://127.0.0.1:8087 \
	go run ./apps/gateway/cmd/gateway

run-inventory:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' \
	OTEL_EXPORTER='$(OTEL_EXPORTER)' OTEL_EXPORTER_OTLP_ENDPOINT='$(OTEL_EXPORTER_OTLP_ENDPOINT)' \
	go run ./apps/inventory/cmd/inventory

run-payroll:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' \
	OTEL_EXPORTER='$(OTEL_EXPORTER)' OTEL_EXPORTER_OTLP_ENDPOINT='$(OTEL_EXPORTER_OTLP_ENDPOINT)' \
	go run ./apps/payroll/cmd/payroll

run-reporting:
	DEV_AUTH_BYPASS=true OPA_URL='$(OPA_URL)' \
	OTEL_EXPORTER='$(OTEL_EXPORTER)' OTEL_EXPORTER_OTLP_ENDPOINT='$(OTEL_EXPORTER_OTLP_ENDPOINT)' \
	INVENTORY_URL=http://127.0.0.1:8082 PAYROLL_URL=http://127.0.0.1:8083 \
	go run ./apps/reporting-bff/cmd/reporting

run-reports:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' \
	OTEL_EXPORTER='$(OTEL_EXPORTER)' OTEL_EXPORTER_OTLP_ENDPOINT='$(OTEL_EXPORTER_OTLP_ENDPOINT)' \
	IMAGE_STORAGE_PATH=./data/image-reports IMAGE_MAX_EDGE=1280 IMAGE_JPEG_QUALITY=82 \
	PUBLIC_WEB_BASE='$(or $(PUBLIC_WEB_BASE),http://localhost:5173)' \
	IMAGE_UPLOAD_SESSION_TTL_MIN='$(or $(IMAGE_UPLOAD_SESSION_TTL_MIN),20)' \
	go run ./apps/reports/cmd/reports

run-search:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' \
	OTEL_EXPORTER='$(OTEL_EXPORTER)' OTEL_EXPORTER_OTLP_ENDPOINT='$(OTEL_EXPORTER_OTLP_ENDPOINT)' \
	SEARCH_ADDR=:8086 SEARCH_REINDEX_SECONDS=30 \
	go run ./apps/search/cmd/search

run-messaging:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' \
	OTEL_EXPORTER='$(OTEL_EXPORTER)' OTEL_EXPORTER_OTLP_ENDPOINT='$(OTEL_EXPORTER_OTLP_ENDPOINT)' \
	MESSAGING_ADDR=:8087 \
	go run ./apps/messaging/cmd/messaging

run-relay:
	DATABASE_URL='$(DATABASE_URL)' NATS_URL='$(NATS_URL)' OUTBOX_POLL_INTERVAL=1s \
	OTEL_EXPORTER='$(OTEL_EXPORTER)' OTEL_EXPORTER_OTLP_ENDPOINT='$(OTEL_EXPORTER_OTLP_ENDPOINT)' \
	go run ./apps/outbox-relay/cmd/relay

run-notification:
	DATABASE_URL='$(DATABASE_URL)' NATS_URL='$(NATS_URL)' \
	OTEL_EXPORTER='$(OTEL_EXPORTER)' OTEL_EXPORTER_OTLP_ENDPOINT='$(OTEL_EXPORTER_OTLP_ENDPOINT)' \
	SMTP_ENABLED=true SMTP_HOST=127.0.0.1 SMTP_PORT=1025 SMTP_FROM=nexus@demo.local \
	NOTIFY_EMAIL_TO=ops@demo.nexus,analyst@demo.nexus \
	go run ./apps/notification/cmd/notification

run-web:
	pnpm --filter @nexus/web dev
